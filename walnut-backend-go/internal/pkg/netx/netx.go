// Package netx 提供与 HTTP 传输层相关的通用工具。
package netx

import (
	"net"
	"strings"
)

// proxyHeaders 是解析真实客户端 IP 时依次尝试的请求头。
//
// ⚠️ 顺序是契约的一部分（Python 端 get_client_ip 就是这个顺序）：
// 多级代理下 X-Forwarded-For 与 X-Real-IP 可能给出不同结果，
// 调换顺序会让操作日志与限流记到不同的 IP 上。
var proxyHeaders = []string{
	"X-Forwarded-For",
	"X-Real-IP",
	"Proxy-Client-IP",
	"WL-Proxy-Client-IP",
}

// HeaderGet 抽象「读请求头」，让 ClientIP 能脱离 gin 单测。
type HeaderGet func(key string) string

// ClientIP 提取客户端 IP，对应 Python 端 app/utils/common_util.py 的 get_client_ip。
//
// # 为什么不直接用 gin 的 c.ClientIP()
//
// 两者算法不同，会给出不同结果：
//   - gin 从 X-Forwarded-For **最右侧**往左找第一个非可信代理的地址
//   - Python 端取 X-Forwarded-For 的**第一个**（最左）逗号分段
//
// 操作日志的 oper_ip 会展示给用户、限流按它计数，故必须与另两端一致，
// 不能因为「gin 的实现更严谨」就换掉——那会造成三端记录不同的 IP。
//
// # 安全语义
//
// 只有当**直连对端地址**在 trusted 白名单内时才解析代理头，
// 否则一律返回对端地址。这样未经认证的客户端无法通过伪造
// X-Forwarded-For 篡改 IP（绕过限流/登录锁定等策略）。
func ClientIP(remoteAddr string, get HeaderGet, trusted []string) string {
	peer := hostFromAddr(remoteAddr)
	if len(trusted) == 0 || !containsExact(trusted, peer) {
		return peer
	}
	if get == nil {
		return peer
	}
	for _, h := range proxyHeaders {
		value := get(h)
		// Python: `if value and value.lower() != "unknown"`
		// ⚠️ 比较的是**原始值**（未 trim），且只排除恰好等于 "unknown" 的情形。
		// " unknown " 会通过判断，随后取第一个逗号分段再 trim —— 保持同样行为。
		if value != "" && !strings.EqualFold(value, "unknown") {
			first, _, _ := strings.Cut(value, ",")
			return strings.TrimSpace(first)
		}
	}
	return peer
}

// hostFromAddr 从 "ip:port" 形式的 RemoteAddr 中取出 IP。
//
// 无端口的裸 IP（少数测试场景）也要能处理，故 SplitHostPort 失败时
// 退回原串——但仅当它本身不含冒号，否则可能是 IPv6 字面量。
func hostFromAddr(remoteAddr string) string {
	addr := strings.TrimSpace(remoteAddr)
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	if !strings.Contains(addr, ":") {
		return addr
	}
	// 形如 "::1" 的裸 IPv6
	if ip := net.ParseIP(addr); ip != nil {
		return ip.String()
	}
	return addr
}

// containsExact 做**精确字符串**匹配，与 Python 端 `peer_ip not in LIST` 一致。
//
// ⚠️ 不支持 CIDR。Python 端也是纯列表成员判断，故 TRUSTED_PROXY_IPS
// 必须填具体 IP（如容器网关地址），填网段不会生效——三端同此限制。
func containsExact(list []string, v string) bool {
	if v == "" {
		return false
	}
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
