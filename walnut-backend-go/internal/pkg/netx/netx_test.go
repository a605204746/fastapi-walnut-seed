package netx

import "testing"

// headers 构造一个固定映射的 HeaderGet。
func headers(m map[string]string) HeaderGet {
	return func(k string) string { return m[k] }
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		hdrs       map[string]string
		trusted    []string
		want       string
	}{
		{
			name:       "白名单为空时一律返回对端地址",
			remoteAddr: "203.0.113.9:54321",
			hdrs:       map[string]string{"X-Forwarded-For": "198.51.100.7"},
			trusted:    nil,
			want:       "203.0.113.9",
		},
		{
			// 这是核心安全断言：非可信对端不得通过伪造头改写 IP
			name:       "对端不在白名单时忽略伪造的XFF",
			remoteAddr: "203.0.113.9:54321",
			hdrs:       map[string]string{"X-Forwarded-For": "198.51.100.7"},
			trusted:    []string{"172.18.0.5"},
			want:       "203.0.113.9",
		},
		{
			name:       "对端可信时取XFF第一段",
			remoteAddr: "172.18.0.5:40000",
			hdrs:       map[string]string{"X-Forwarded-For": "198.51.100.7, 203.0.113.9"},
			trusted:    []string{"172.18.0.5"},
			want:       "198.51.100.7",
		},
		{
			name:       "XFF第一段带空白需trim",
			remoteAddr: "172.18.0.5:40000",
			hdrs:       map[string]string{"X-Forwarded-For": "  198.51.100.7 ,203.0.113.9"},
			trusted:    []string{"172.18.0.5"},
			want:       "198.51.100.7",
		},
		{
			name:       "XFF为unknown时落到下一个头",
			remoteAddr: "172.18.0.5:40000",
			hdrs:       map[string]string{"X-Forwarded-For": "unknown", "X-Real-IP": "198.51.100.7"},
			trusted:    []string{"172.18.0.5"},
			want:       "198.51.100.7",
		},
		{
			name:       "UNKNOWN大小写不敏感",
			remoteAddr: "172.18.0.5:40000",
			hdrs:       map[string]string{"X-Forwarded-For": "UnKnown", "X-Real-IP": "198.51.100.7"},
			trusted:    []string{"172.18.0.5"},
			want:       "198.51.100.7",
		},
		{
			// Python 比较的是原始值，" unknown " 不等于 "unknown"，故会被采用
			name:       "带空白的unknown不算unknown（与Python同行为）",
			remoteAddr: "172.18.0.5:40000",
			hdrs:       map[string]string{"X-Forwarded-For": " unknown "},
			trusted:    []string{"172.18.0.5"},
			want:       "unknown",
		},
		{
			name:       "头的优先级XFF高于XRealIP",
			remoteAddr: "172.18.0.5:40000",
			hdrs:       map[string]string{"X-Forwarded-For": "198.51.100.1", "X-Real-IP": "198.51.100.2"},
			trusted:    []string{"172.18.0.5"},
			want:       "198.51.100.1",
		},
		{
			name:       "回退到Proxy-Client-IP与WL-Proxy-Client-IP",
			remoteAddr: "172.18.0.5:40000",
			hdrs:       map[string]string{"WL-Proxy-Client-IP": "198.51.100.4"},
			trusted:    []string{"172.18.0.5"},
			want:       "198.51.100.4",
		},
		{
			name:       "可信对端但无任何代理头时返回对端地址",
			remoteAddr: "172.18.0.5:40000",
			hdrs:       nil,
			trusted:    []string{"172.18.0.5"},
			want:       "172.18.0.5",
		},
		{
			name:       "IPv6对端地址",
			remoteAddr: "[::1]:54321",
			hdrs:       map[string]string{"X-Forwarded-For": "198.51.100.7"},
			trusted:    nil,
			want:       "::1",
		},
		{
			name:       "CIDR写在白名单里不生效（三端同此限制）",
			remoteAddr: "172.18.0.5:40000",
			hdrs:       map[string]string{"X-Forwarded-For": "198.51.100.7"},
			trusted:    []string{"172.18.0.0/16"},
			want:       "172.18.0.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClientIP(tt.remoteAddr, headers(tt.hdrs), tt.trusted); got != tt.want {
				t.Errorf("ClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHostFromAddr(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1:8011": "127.0.0.1",
		"[::1]:8011":     "::1",
		"::1":            "::1",
		"127.0.0.1":      "127.0.0.1",
		"":               "",
		" 10.0.0.1:80 ":  "10.0.0.1",
	}
	for in, want := range cases {
		if got := hostFromAddr(in); got != want {
			t.Errorf("hostFromAddr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClientIPNilGetter(t *testing.T) {
	// 可信对端但没有取头函数时不得 panic，应回退到对端地址
	got := ClientIP("172.18.0.5:40000", nil, []string{"172.18.0.5"})
	if got != "172.18.0.5" {
		t.Errorf("got=%q want=172.18.0.5", got)
	}
}
