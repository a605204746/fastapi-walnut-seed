package cryptox

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// BcryptCost 生成新哈希时使用的 cost。
//
// ⚠️ 必须是 **12**，不能用 bcrypt.DefaultCost（=10）。
// Python 端全部三处新建密码都调 `bcrypt.hashpw(pwd, bcrypt.gensalt())`，
// 而 bcrypt 库的 `gensalt()` 无参默认 rounds=**12**。
// Go 若用 DefaultCost=10，新建账号的哈希强度就比另两端弱一档。
//
// 实测确认（2026-11-02）：
//   - 种子数据里的哈希是 `$2a$10$...`（cost 10，历史预生成，两库逐字节相同）
//   - Python 新建用户产生 cost 12
//
// 校验时 cost 从哈希串自身读取，故两种都能验；差异只影响**新写入**的哈希。
const BcryptCost = 12

// bcrypt 哈希前缀。x/crypto/bcrypt 生成 `$2a$`，Python 的 bcrypt 库生成 `$2b$`。
// 两者算法等价（2b 只是修了一个超长密码的长度溢出 bug），且各实现都互相接受
// 2a/2b/2y，因此**跨端校验没有问题**，只是新建哈希的前缀字符不同。
// Go 标准库无导出手段生成 2b，故这是一处**已接受的显示层差异**（见实施计划 R9）。
const (
	prefix2a = "$2a$"
	prefix2b = "$2b$"
	prefix2y = "$2y$"
)

// HashPassword 生成 bcrypt 哈希。对应 Python 端 bcrypt.hashpw(pwd, bcrypt.gensalt())。
//
// ⚠️ bcrypt 只处理密码的**前 72 字节**，超出部分被静默截断——两端行为一致
// （Python 的 bcrypt 库同样截断），故不需要在 Go 侧额外报错，
// 但调用方不应依赖 72 字节之后的熵。
func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("生成密码哈希失败: %w", err)
	}
	return string(hash), nil
}

// CheckPassword 校验明文与哈希是否匹配。对应 Python 端 bcrypt.checkpw。
//
// 返回 bool 而非 error：调用方（登录流程）只关心「对不对」，
// 哈希格式非法与密码错误应当走**同一条**失败分支——这也符合
// 「用户不存在与密码错误统一文案」的防账号枚举要求。
// 需要区分二者的场景用 CheckPasswordStrict。
//
// ⚠️ 必须同时判 ok 与 err：CheckPasswordStrict 在「密码错误」时返回 (false, nil)
// ——密码不对是正常业务结果而非 error。若只看 `err == nil` 就返回 true，
// 会导致**任何密码都能通过校验**（认证绕过）。此坑已被
// TestCheckPassword / TestCheckPasswordAcceptsSeedHash 锁住。
func CheckPassword(hashed, plain string) bool {
	ok, err := CheckPasswordStrict(hashed, plain)
	return ok && err == nil
}

// CheckPasswordStrict 校验并区分「密码错误」与「哈希本身非法」。
//
// 需要区分的场景：修改密码接口要判断旧密码是否正确（错误 → 业务提示），
// 而哈希非法意味着数据损坏（应记日志并当作认证失败，不该提示"密码错误"）。
func CheckPasswordStrict(hashed, plain string) (bool, error) {
	if strings.TrimSpace(hashed) == "" {
		return false, fmt.Errorf("密码哈希为空")
	}
	if !hasKnownPrefix(hashed) {
		return false, fmt.Errorf("密码哈希前缀无法识别（接受 %s/%s/%s）: %.7s...",
			prefix2a, prefix2b, prefix2y, hashed)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain)); err != nil {
		// ErrMismatchedHashAndPassword 是"密码不对"，属正常业务结果，
		// 不当作 error 上抛——否则调用方无法区分"密码错"与"哈希坏了"
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// hasKnownPrefix 判断是否为可识别的 bcrypt 变体前缀。
func hasKnownPrefix(h string) bool {
	return strings.HasPrefix(h, prefix2a) ||
		strings.HasPrefix(h, prefix2b) ||
		strings.HasPrefix(h, prefix2y)
}

// HashCost 从哈希串解析 cost，用于诊断与测试断言。解析失败返回 0。
func HashCost(hashed string) int {
	parts := strings.Split(hashed, "$")
	// bcrypt 格式：$2b$12$<22 字符 salt><31 字符 hash>
	if len(parts) < 4 {
		return 0
	}
	var cost int
	if _, err := fmt.Sscanf(parts[2], "%d", &cost); err != nil {
		return 0
	}
	return cost
}
