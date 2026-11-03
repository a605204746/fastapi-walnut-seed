package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"walnut-seed-go/internal/config"
	"walnut-seed-go/internal/module/system/client"
	logmodel "walnut-seed-go/internal/module/system/log"
	"walnut-seed-go/internal/module/system/user"
	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/captcha"
	"walnut-seed-go/internal/pkg/cryptox"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/i18nx"
	"walnut-seed-go/internal/pkg/idgen"
	"walnut-seed-go/internal/pkg/modelx"
)

// Redis 键前缀段，对应 Python 端 CacheNames。
//
// ⚠️ 这些是**相对键**，cachex.Client.Key() 会自动叠加 REDIS_KEY_PREFIX 根前缀。
// 不要在业务代码里自己拼根前缀——那会导致双重前缀，与另两端的键完全对不上。
const (
	keyCaptchaCode  = "global:captcha_codes:"
	keyPwdErrCount  = "pwd_err_cnt:"
	keyOnlineToken  = "online_tokens:"
	keyUserSession  = "user_session:"
	keyAccessToken  = "access_token:"
	keyLoginTypePwd = "password"
)

// SessionInfo 存入 Redis 的会话快照。
//
// ⚠️ JSON 键名用 **snake_case**，与 Python 端 `json.dumps(session_info)` 一致
// （Python 那边是普通 dict，键就是 user_id/user_name/...）。
// 鉴权中间件（P2 的 get_current_user）与在线用户页都要读它，键名不一致会静默取到零值。
type SessionInfo struct {
	UserID         int64    `json:"user_id"`
	UserName       string   `json:"user_name"`
	Nickname       string   `json:"nickname"`
	DeptID         *int64   `json:"dept_id"`
	DeptName       string   `json:"dept_name"`
	DeptCategory   string   `json:"dept_category"`
	UserStatus     string   `json:"user_status"`
	MenuPermission []string `json:"menu_permission"`
	RolePermission []string `json:"role_permission"`
	MenuIDs        []int64  `json:"menu_ids"`
	// ActiveTimeout 活跃超时（秒），鉴权续期时用，避免依赖全局默认值。
	//
	// ⚠️ 必须是**指针**：Python 端读的是
	// `session_info.get("_active_timeout")` 并区分 None 与 0——
	// None（旧会话没有该字段）→ **不续期**；0 或负数 → 直接判"认证已失效"。
	// 用值类型 int 时两者都是 0，会把"不续期"错判成"拒绝访问"。
	ActiveTimeout *int `json:"_active_timeout"`
}

// OnlineUser 在线用户信息（写入 online_tokens:{token}）。
//
// ⚠️ 这里的键名是 **camelCase**（tokenId/userName/ipaddr…），
// 与 SessionInfo 的 snake_case 不同——Python 端 _build_online_dto 返回的就是驼峰 dict。
// 两处不一致是既有事实，照抄即可。
type OnlineUser struct {
	TokenID       string `json:"tokenId"`
	UserName      string `json:"userName"`
	DeptName      string `json:"deptName"`
	IPAddr        string `json:"ipaddr"`
	LoginLocation string `json:"loginLocation"`
	Browser       string `json:"browser"`
	OS            string `json:"os"`
	// LoginTime 毫秒时间戳（Python 端 int(datetime.now().timestamp() * 1000)）
	LoginTime  int64  `json:"loginTime"`
	ClientKey  string `json:"clientKey"`
	DeviceType string `json:"deviceType"`
}

// Service 认证服务，对应 Python 端 AuthService。
type Service struct {
	repo   *Repo
	db     *gorm.DB
	cache  *cachex.Client
	cfg    *config.Config
	logger *slog.Logger
	locale string
}

// NewService 构造认证服务。locale 来自请求（i18n 文案需要）。
func NewService(db *gorm.DB, cache *cachex.Client, cfg *config.Config, logger *slog.Logger, locale string) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:   NewRepo(db),
		db:     db,
		cache:  cache,
		cfg:    cfg,
		logger: logger,
		locale: locale,
	}
}

// t 按请求语言解析 i18n 文案。
func (s *Service) t(key string, args ...any) string {
	return i18nx.MessageIn(s.locale, key, args...)
}

// RequestMeta 一次请求的上下文信息（IP、UA、token 头等）。
//
// 抽成结构体而不是直接传 *gin.Context：service 层不应依赖 HTTP 框架，
// 否则无法脱离 gin 做单元测试。
type RequestMeta struct {
	ClientIP  string
	UserAgent string
	// Authorization 原始请求头值（登出时需要从中取 token）
	Authorization string
}

// ==================== 登录 ====================

// LoginPassword 密码登录，对应 Python 端 AuthService.login_password。
//
// 核心安全语义（必须与 Python 端逐条一致）：
//  1. **用户不存在与密码错误走同一路径**：同样计入重试次数、返回同一错误码与文案，
//     不回显"用户不存在"——否则攻击者可以枚举出哪些账号存在
//  2. 账号被停用在密码校验**之前**就拒绝（Python 端顺序如此）
//  3. 重试计数键是「用户名 + IP」组合，防止攻击者用错误密码恶意锁定他人账号（DoS）
func (s *Service) LoginPassword(ctx context.Context, body *PasswordLoginRequest, meta RequestMeta, cli *client.SysClient) (*LoginResponse, error) {
	username, password := body.Username, body.Password

	// ---- 1. 验证码 ----
	if s.cfg.CaptchaEnable {
		if err := s.validateCaptcha(ctx, username, strDeref(body.Code), strDeref(body.UUID)); err != nil {
			return nil, err
		}
	}

	// ---- 2. 查用户 ----
	u, err := s.repo.FindUserByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	// 停用账号在密码校验前就拒绝（与 Python 端顺序一致）
	if u != nil && u.Status == modelx.Disabled {
		s.logger.Info("登录用户已被停用", slog.String("username", username))
		return nil, errs.FromAuthCode(errs.AuthUserBlocked)
	}

	// ---- 3. 密码校验 ----
	// mismatch 默认为 true：**用户不存在时也算一次失败**并计入重试，
	// 这样"用户不存在"与"密码错误"在外部完全不可区分（防枚举）
	mismatch := true
	if u != nil {
		hash := ""
		if u.Password != nil {
			hash = *u.Password
		}
		// CheckPassword 内部已处理 bcrypt 格式非法的情况（返回 false 而非 panic）
		mismatch = !cryptox.CheckPassword(hash, password)
	} else {
		s.logger.Info("登录用户不存在", slog.String("username", username))
	}

	// ---- 4. 重试计数校验（失败时在此抛错）----
	if err := s.checkLogin(ctx, keyLoginTypePwd, username, meta.ClientIP, mismatch); err != nil {
		return nil, err
	}

	// ---- 5. 构建会话并发放 token ----
	session, err := s.buildSessionInfo(ctx, u)
	if err != nil {
		return nil, err
	}
	resp, err := s.doLogin(ctx, u, session, cli, meta)
	if err != nil {
		return nil, err
	}

	// ---- 6. 成功日志与最后登录信息 ----
	s.recordLogininfor(ctx, meta, username, modelx.LoginSuccess, s.t("user.login.success"), logmodel.StatusSuccess, cli)
	if err := s.repo.UpdateLoginInfo(ctx, u.ID, meta.ClientIP, time.Now()); err != nil {
		// 更新登录信息失败不应让登录本身失败——token 已发放，回滚反而更糟
		s.logger.Warn("更新最后登录信息失败", slog.String("username", username), slog.String("err", err.Error()))
	}
	return resp, nil
}

// validateCaptcha 校验图形验证码，对应 Python 端 AuthService.validate_captcha。
//
// ⚠️ **无论校验成功与否都要删除 Redis 里的验证码**：
// 一次性凭证复用等于没有验证码。Python 端在读取后立即 delete，这里同样。
func (s *Service) validateCaptcha(ctx context.Context, username, code, uuid string) error {
	key := keyCaptchaCode + uuid
	stored, ok := s.cache.Get(ctx, key)
	// 先删再用：即使后面校验失败，这个 uuid 也已作废
	s.cache.Delete(ctx, key)

	if !ok || stored == "" {
		s.recordLogininfor(ctx, RequestMeta{ClientIP: ""}, username, modelx.LoginFail,
			s.t("user.jcaptcha.expire"), logmodel.StatusFail, nil)
		return errs.FromAuthCode(errs.AuthCaptchaExpired)
	}
	if !captcha.EqualsIgnoreCase(code, stored) {
		s.recordLogininfor(ctx, RequestMeta{ClientIP: ""}, username, modelx.LoginFail,
			s.t("user.jcaptcha.error"), logmodel.StatusFail, nil)
		return errs.FromAuthCode(errs.AuthCaptchaError)
	}
	return nil
}

// checkLogin 登录重试次数校验，对应 Python 端 AuthService.check_login。
//
// 锁定键是「用户名 + IP」组合：若只按用户名，攻击者可以用错误密码
// 把任意账号锁死（拒绝服务）。加上 IP 后，锁定的只是"这个 IP 对这个账号"的组合。
func (s *Service) checkLogin(ctx context.Context, loginType, username, ip string, mismatch bool) error {
	if ip == "" {
		ip = "unknown"
	}
	errorKey := fmt.Sprintf("%s%s:%s", keyPwdErrCount, username, ip)
	maxRetry := int64(s.cfg.PasswordMaxRetryCount)
	lockTime := time.Duration(s.cfg.PasswordLockTime) * time.Minute
	retryLimitMsgKey := retryLimitKeyFor(loginType)

	count := s.getErrCount(ctx, errorKey)
	if count >= maxRetry {
		s.recordLogininfor(ctx, RequestMeta{ClientIP: ip}, username, modelx.LoginFail,
			s.t(retryLimitMsgKey, maxRetry, s.cfg.PasswordLockTime), logmodel.StatusFail, nil)
		return errs.FromAuthCode(errs.AuthPasswordRetryExceeded, maxRetry, s.cfg.PasswordLockTime)
	}

	if mismatch {
		// IncrWithTTL 每次失败都刷新 TTL：与 Python 端 `set(key, n, expire=lock_time*60)`
		// 的语义一致（滑动窗口，而非从首次失败起固定计时）
		newCount, ok := s.cache.IncrWithTTL(ctx, errorKey, lockTime)
		if !ok {
			newCount = count + 1
		}
		if newCount >= maxRetry {
			s.recordLogininfor(ctx, RequestMeta{ClientIP: ip}, username, modelx.LoginFail,
				s.t(retryLimitMsgKey, maxRetry, s.cfg.PasswordLockTime), logmodel.StatusFail, nil)
			return errs.FromAuthCode(errs.AuthPasswordRetryExceeded, maxRetry, s.cfg.PasswordLockTime)
		}
		s.recordLogininfor(ctx, RequestMeta{ClientIP: ip}, username, modelx.LoginFail,
			s.t("user.password.not.match"), logmodel.StatusFail, nil)
		return errs.FromAuthCode(errs.AuthUserPasswordNotMatch)
	}

	// 登录成功 → 清空失败计数
	s.cache.Delete(ctx, errorKey)
	return nil
}

// getErrCount 读取当前失败次数；键不存在或解析失败均按 0 处理。
func (s *Service) getErrCount(ctx context.Context, key string) int64 {
	raw, ok := s.cache.Get(ctx, key)
	if !ok || raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		// 计数值损坏时按 0 处理并清理，避免用户被一个坏值永久锁住
		s.logger.Warn("登录失败计数值非法，已重置", slog.String("key", key), slog.String("raw", raw))
		s.cache.Delete(ctx, key)
		return 0
	}
	return n
}

// retryLimitKeyFor 按登录类型返回重试提示的 i18n 键，对应 LoginType.retry_limit_key。
func retryLimitKeyFor(loginType string) string {
	switch loginType {
	case keyLoginTypePwd:
		return "user.password.retry.limit.exceed"
	case "sms":
		return "sms.code.retry.limit.exceed"
	case "email":
		return "email.code.retry.limit.exceed"
	default:
		return ""
	}
}

// buildSessionInfo 构建会话信息（用户基本信息 + 角色权限 + 菜单权限）。
//
// 对应 Python 端 AuthService.build_session_info。超管走特殊分支：
// role_permission=["superadmin"]、menu_permission=["*:*:*"]、menu_ids=全部有效菜单。
func (s *Service) buildSessionInfo(ctx context.Context, u *user.SysUser) (*SessionInfo, error) {
	isSuper := u.ID == modelx.SuperAdminID

	deptName, deptCategory := "", ""
	if u.DeptID != nil && *u.DeptID != 0 {
		d, err := s.repo.FindDeptByID(ctx, *u.DeptID)
		if err != nil {
			return nil, fmt.Errorf("查询部门失败: %w", err)
		}
		if d != nil {
			deptName = strDeref(d.DeptName)
			deptCategory = strDeref(d.DeptCategory)
		}
	}

	info := &SessionInfo{
		UserID:       u.ID,
		UserName:     u.UserName,
		Nickname:     u.NickName,
		DeptID:       u.DeptID,
		DeptName:     deptName,
		DeptCategory: deptCategory,
		UserStatus:   u.Status,
		// 初始化为空切片而非 nil：序列化后是 [] 而不是 null，
		// 前端遍历权限数组时不会因 null 报错
		MenuPermission: []string{},
		RolePermission: []string{},
		MenuIDs:        []int64{},
	}

	if isSuper {
		info.RolePermission = []string{modelx.SuperAdminRoleKey}
		info.MenuPermission = []string{modelx.AllPermission}
		ids, err := s.repo.FindAllActiveMenuIDs(ctx)
		if err != nil {
			return nil, fmt.Errorf("查询超管菜单失败: %w", err)
		}
		info.MenuIDs = ids
		return info, nil
	}

	roles, err := s.repo.FindActiveRolesByUserID(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("查询用户角色失败: %w", err)
	}
	roleKeys := make([]string, 0, len(roles))
	roleIDs := make([]int64, 0, len(roles))
	for _, r := range roles {
		if r.RoleKey != "" {
			roleKeys = append(roleKeys, r.RoleKey)
		}
		roleIDs = append(roleIDs, r.ID)
	}
	info.RolePermission = roleKeys

	if len(roleIDs) > 0 {
		menus, err := s.repo.FindMenusByRoleIDs(ctx, roleIDs)
		if err != nil {
			return nil, fmt.Errorf("查询角色菜单失败: %w", err)
		}
		// 去重 + 排序：与 Python 端 `sorted({m.id for m in menus})` 一致。
		// 一个用户可能有多个角色，菜单会重复；顺序不定会让会话快照的 JSON
		// 每次都不一样，既不利于比对也浪费缓存
		idSet := make(map[int64]struct{}, len(menus))
		permSet := make(map[string]struct{}, len(menus))
		for _, m := range menus {
			idSet[m.ID] = struct{}{}
			if m.Perms != nil {
				if p := strings.TrimSpace(*m.Perms); p != "" {
					permSet[p] = struct{}{}
				}
			}
		}
		info.MenuIDs = sortedKeys(idSet)
		info.MenuPermission = sortedStringKeys(permSet)
	}
	return info, nil
}

// doLogin 发放 JWT 并写入会话/在线缓存，对应 Python 端 AuthService.do_login。
func (s *Service) doLogin(ctx context.Context, u *user.SysUser, session *SessionInfo, cli *client.SysClient, meta RequestMeta) (*LoginResponse, error) {
	sessionID := idgen.UUID()

	timeout := derefInt(cli.Timeout, s.cfg.AccessTokenExpireSeconds)
	activeTimeout := derefInt(cli.ActiveTimeout, timeout)
	if timeout <= 0 || activeTimeout <= 0 {
		return nil, errs.New("客户端 token 超时时间配置无效")
	}
	// 活跃超时不能超过固定超时，否则 Redis 会话可能比 JWT 硬过期时间活得更久，
	// 出现"token 已过期但会话还在"的矛盾状态
	if activeTimeout > timeout {
		activeTimeout = timeout
	}

	// JWT 的 exp 始终是**固定硬过期**；滑动过期只通过 Redis TTL 实现。
	// 这是刻意设计：JWT 无法撤销，若把 exp 也滑动续期，被盗的 token 就永远有效。
	uid := u.ID
	uname := u.UserName
	clientID := strDeref(cli.ClientID)
	// ⚠️ dept_name / dept_category 用**空字符串**而非 null。
	// Python 端 build_session_info 里是 `dept.dept_name or ""`，得到的是 ""，
	// 再经 session_info.get() 传入 JWTPayloadSchema，故 claims 里是 "" 不是 null。
	// 曾误用 nilIfEmpty 转成 null，被认证契约比对抓出（py='' vs go=None）。
	deptName := session.DeptName
	deptCategory := session.DeptCategory
	claims := cryptox.Claims{
		Sub:          sessionID,
		UserID:       &uid,
		UserName:     &uname,
		DeptID:       session.DeptID,
		DeptName:     &deptName,
		DeptCategory: &deptCategory,
		ClientID:     &clientID,
		IsRefresh:    false,
		Exp:          cryptox.ExpiresAt(time.Duration(timeout) * time.Second),
	}
	token, err := cryptox.SignToken(claims, s.cfg.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("签发 token 失败: %w", err)
	}

	// 活跃超时写入会话，鉴权续期时不依赖全局默认值
	session.ActiveTimeout = &activeTimeout
	ttl := time.Duration(timeout) * time.Second
	if s.cfg.TokenSlidingExpire {
		ttl = time.Duration(activeTimeout) * time.Second
	}

	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("序列化会话失败: %w", err)
	}
	s.cache.Set(ctx, keyUserSession+sessionID, string(sessionJSON), ttl)
	s.cache.Set(ctx, keyAccessToken+sessionID, token, ttl)

	online, err := s.buildOnlineUser(u, session, cli, token, meta)
	if err != nil {
		return nil, err
	}
	onlineJSON, err := json.Marshal(online)
	if err != nil {
		return nil, fmt.Errorf("序列化在线用户信息失败: %w", err)
	}
	s.cache.Set(ctx, keyOnlineToken+token, string(onlineJSON), ttl)

	return &LoginResponse{
		AccessToken:     token,
		RefreshToken:    nil,
		ExpireIn:        timeout,
		RefreshExpireIn: nil,
		ClientID:        clientID,
		Scope:           nil,
		OpenID:          nil,
	}, nil
}

// buildOnlineUser 构建在线用户信息，对应 Python 端 _build_online_dto。
func (s *Service) buildOnlineUser(u *user.SysUser, session *SessionInfo, cli *client.SysClient, token string, meta RequestMeta) (*OnlineUser, error) {
	browser, osName := parseUserAgent(meta.UserAgent)
	return &OnlineUser{
		TokenID:       token,
		UserName:      u.UserName,
		DeptName:      session.DeptName,
		IPAddr:        meta.ClientIP,
		LoginLocation: locateIP(meta.ClientIP),
		Browser:       browser,
		OS:            osName,
		LoginTime:     time.Now().UnixMilli(),
		ClientKey:     strDeref(cli.ClientKey),
		DeviceType:    strDeref(cli.DeviceType),
	}, nil
}

// ==================== 登出 ====================

// Logout 退出登录，对应 Python 端 AuthService.logout。
//
// **幂等**：token 无效/已过期也返回成功。这是 /auth/logout 在认证白名单内的原因——
// 未登录或 token 失效时调用它不应报错，否则前端"退出"按钮在 token 过期后会卡住。
func (s *Service) Logout(ctx context.Context, meta RequestMeta) error {
	token := extractBearerToken(meta.Authorization, s.cfg.TokenName, s.cfg.TokenPrefix)
	if token == "" {
		return nil
	}

	username := ""
	// 解析时 verifyExp=false：登出时 token 可能刚好过期，但仍要清掉它的会话，
	// 否则 Redis 里会留下一个再也无法访问的僵尸会话
	claims, err := cryptox.ParseToken(token, s.cfg.SecretKey, false)
	if err == nil {
		sessionID := claims.Sub
		if claims.UserName != nil {
			username = *claims.UserName
		}
		if raw, ok := s.cache.Get(ctx, keyUserSession+sessionID); ok && raw != "" {
			var sess SessionInfo
			if jsonErr := json.Unmarshal([]byte(raw), &sess); jsonErr == nil && sess.UserName != "" {
				username = sess.UserName
			}
		}
		s.cache.Delete(ctx, keyUserSession+sessionID, keyAccessToken+sessionID)
	} else {
		// 解析失败只记 debug 日志，不影响登出成功（与 Python 端 except 后 pass 一致）
		s.logger.Debug("登出时解析 token 失败，按幂等成功处理", slog.String("err", err.Error()))
	}
	s.cache.Delete(ctx, keyOnlineToken+token)

	if username != "" {
		s.recordLogininfor(ctx, meta, username, modelx.Logout, s.t("user.logout.success"), logmodel.StatusSuccess, nil)
	}
	return nil
}

// ==================== 注册 ====================

// Register 用户注册，对应 Python 端 AuthService.register。
func (s *Service) Register(ctx context.Context, body *RegisterRequest, meta RequestMeta) error {
	username, password := body.Username, body.Password

	userType := strDeref(body.UserType)
	if userType == "" {
		userType = UserTypeSys
	}
	if !isValidUserType(userType) {
		return errs.Newf("用户类型 %q 非法", userType)
	}

	if s.cfg.CaptchaEnable {
		if err := s.validateCaptcha(ctx, username, strDeref(body.Code), strDeref(body.UUID)); err != nil {
			return err
		}
	}

	n, err := s.repo.CountActiveUsersByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("查询同名用户失败: %w", err)
	}
	if n > 0 {
		return errs.FromAuthCode(errs.AuthUserRegisterExists, username)
	}

	hashed, err := cryptox.HashPassword(password)
	if err != nil {
		return fmt.Errorf("生成密码哈希失败: %w", err)
	}

	// create_by / update_by 用 -1 表示"系统自建"（注册无操作人），与 Python 端一致
	sysUser := int64(-1)
	now := time.Now()
	newUser := &user.SysUser{
		UserName: username,
		NickName: username,
		Password: &hashed,
		UserType: userType,
		Status:   modelx.Normal,
		DelFlag:  modelx.Normal,
	}
	// ⚠️ 以下字段来自嵌入的 modelx.BaseModel，**不能写进上面的复合字面量**
	// （Go 的复合字面量不接受提升字段名，会报 unknown field），
	// 但逐字段赋值可以用提升名访问。
	newUser.ID = idgen.NextID()
	newUser.CreateBy = &sysUser
	newUser.UpdateBy = &sysUser
	newUser.CreateTime = &now

	if err := s.repo.CreateUser(ctx, newUser); err != nil {
		// 并发注册同名账号时会撞唯一约束。此时应报"已存在"而不是暴露 SQL 错误
		if isDuplicate(err) {
			return errs.FromAuthCode(errs.AuthUserRegisterExists, username)
		}
		return fmt.Errorf("创建用户失败: %w", err)
	}

	s.recordLogininfor(ctx, meta, username, modelx.Register, s.t("user.register.success"), logmodel.StatusSuccess, nil)
	return nil
}

// 用户类型取值，对应 Python 端 UserType 枚举。
const (
	UserTypeSys = "sys_user"
	UserTypeApp = "app_user"
)

func isValidUserType(v string) bool { return v == UserTypeSys || v == UserTypeApp }

// ==================== 验证码 ====================

// GetCaptcha 生成图形验证码，对应 Python 端 AuthService.get_captcha。
func (s *Service) GetCaptcha(ctx context.Context) (*CaptchaResponse, error) {
	if !s.cfg.CaptchaEnable {
		// 关闭时只返回 captchaEnabled=false，uuid/img 为 null，
		// 前端据此不渲染验证码输入框
		return &CaptchaResponse{CaptchaEnabled: false}, nil
	}

	result, err := captcha.Generate(s.cfg.CaptchaType, s.cfg.CaptchaNumberLength, s.cfg.CaptchaCharLength)
	if err != nil {
		return nil, fmt.Errorf("生成验证码失败: %w", err)
	}

	uuid := idgen.UUID()
	ttl := time.Duration(s.cfg.CaptchaExpireSeconds) * time.Second
	// 存**答案**而非展示文本。math 模式下两者不同（"3+5=?" vs "8"）
	if !s.cache.Set(ctx, keyCaptchaCode+uuid, result.Answer, ttl) {
		// Redis 写不进去意味着这个验证码永远无法校验通过，
		// 与其发给用户一个必然失败的验证码，不如直接报错
		return nil, errs.New("验证码服务不可用，请稍后重试")
	}
	return &CaptchaResponse{CaptchaEnabled: true, UUID: &uuid, Img: &result.ImageBase64}, nil
}

// ==================== 登录日志 ====================

// recordLogininfor 写登录日志，对应 Python 端 AuthService.record_logininfor。
//
// ⚠️ **失败只记日志、不向上返回 error**：登录日志是旁路记录，
// 写不进去不应该让用户登录失败。Python 端用 try/except 包了整个调用，
// 这里同样吞掉错误——但必须记日志，否则日志表静默变空会很难发现。
func (s *Service) recordLogininfor(ctx context.Context, meta RequestMeta, username, status, message, statusCode string, cli *client.SysClient) {
	if s.db == nil {
		return
	}
	entry := &logmodel.SysLogininfor{
		ID:       idgen.NextID(),
		UserName: &username,
		IPAddr:   &meta.ClientIP,
		Status:   &statusCode,
		Msg:      &message,
	}
	now := time.Now()
	entry.LoginTime = &now

	if meta.UserAgent != "" {
		browser, osName := parseUserAgent(meta.UserAgent)
		entry.Browser = &browser
		entry.OS = &osName
	}
	if cli != nil {
		if ck := strDeref(cli.ClientKey); ck != "" {
			entry.ClientKey = &ck
		}
		if dt := strDeref(cli.DeviceType); dt != "" {
			entry.DeviceType = &dt
		}
	}
	entry.LoginLocation = ptrOrNil(locateIP(meta.ClientIP))

	if err := s.repo.InsertLogininfor(ctx, entry); err != nil {
		s.logger.Error("登录日志写入失败",
			slog.String("username", username),
			slog.String("status", status),
			slog.String("err", err.Error()))
	}
	_ = status // status 仅用于日志上下文，入库的是 statusCode
}

// ==================== 辅助函数 ====================

// extractBearerToken 从 Authorization 头取 token。
//
// 同时兼容「token 直接放在查询参数」的形态：SSE 用 EventSource，
// 无法设置自定义请求头，前端只能把 token 放查询串（见 Python 端
// CustomOAuth2PasswordBearer 的同款兼容处理）。
func extractBearerToken(authorization, tokenName, tokenPrefix string) string {
	if authorization == "" {
		return ""
	}
	if idx := strings.Index(authorization, " "); idx >= 0 {
		scheme := authorization[:idx]
		if !strings.EqualFold(scheme, tokenPrefix) {
			return ""
		}
		return strings.TrimSpace(authorization[idx+1:])
	}
	// 没有 scheme 前缀时按裸 token 处理
	return strings.TrimSpace(authorization)
}

// locateIP 返回 IP 的归属地描述，对应 Python 端 get_real_address_by_ip。
//
// 未接入离线 IP 库时，内网地址返回"内网IP"、公网地址返回空串——
// 与 Python 端在无 IP 库情况下的行为一致。P4 可接入真实 IP 库。
func locateIP(ip string) string {
	if ip == "" {
		return ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() {
		return "内网IP"
	}
	return ""
}

// parseUserAgent 从 UA 串提取浏览器与操作系统名。
//
// Python 端用 ua_parser 库做完整解析。这里用**有序的关键字匹配**做轻量实现：
// 在线用户页只需要"Chrome / Windows"这种粒度，不值得为此引入一个 UA 解析库
// 及其正则规则集。顺序很重要——Edge 的 UA 里同时含 "Chrome"，
// 必须先判 Edge 否则会被误认成 Chrome。
func parseUserAgent(ua string) (browser, os string) {
	if ua == "" {
		return "", ""
	}
	switch {
	case strings.Contains(ua, "Edg/"), strings.Contains(ua, "Edge"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/"), strings.Contains(ua, "Opera"):
		browser = "Opera"
	case strings.Contains(ua, "Firefox"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari"):
		browser = "Safari"
	default:
		browser = "Unknown"
	}

	switch {
	case strings.Contains(ua, "Windows"):
		os = "Windows"
	case strings.Contains(ua, "Android"):
		os = "Android"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"), strings.Contains(ua, "iOS"):
		os = "iOS"
	case strings.Contains(ua, "Mac OS"):
		os = "Mac OS"
	case strings.Contains(ua, "Linux"):
		os = "Linux"
	default:
		os = "Unknown"
	}
	return browser, os
}

// isDuplicate 判断是否为唯一键冲突。
//
// 依赖 gorm.Config.TranslateError=true（见 databasex.Open），
// 它会把 MySQL 的 1062 错误翻译成 gorm.ErrDuplicatedKey。
func isDuplicate(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}

func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(p *int, def int) int {
	if p == nil || *p == 0 {
		return def
	}
	return *p
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func sortedKeys(m map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// 排序保证会话快照的 JSON 稳定（与 Python 端 sorted({...}) 一致）。
	// map 遍历顺序随机，不排序会让同一用户的会话 JSON 每次都不同，
	// 既不利于跨端比对，也让 Redis 里的值无法做幂等判断。
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedStringKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
