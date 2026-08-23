// 安全前置（v0.1.8，roadmap M0-0.1）：
//   serve 长期跑在 127.0.0.1 上，防两类攻击：
//     1. DNS rebinding / Host 头欺骗 → Host 校验：只接受回环地址
//        （127.0.0.1 / localhost / ::1），其余 403。
//     2. 恶意网页 CSRF 调用（POST /api/refresh、/api/touch 改状态；
//        读接口会泄露本地笔记）→ token 鉴权：所有 /api/* 请求必须带
//        X-Seren-Token 头或 ?token= 查询参数，与 Server.Token 常量时间比较。
//        token 由服务端注入到页面（handleIndex 替换 __SEREN_TOKEN__ 占位符），
//        外部页面受同源策略限制读不到 → 拿不到 token → 无法调用。
//     3. GET /（页面本身）不需要 token——它是注入源；Server.Token 为空时
//        视为"未配置鉴权"（兼容嵌入式用法），cmd/seren 永远生成非空 token。
package web

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// tokenHeader 前端注入后所有 fetch 自动携带的鉴权头。
const tokenHeader = "X-Seren-Token"

// auth 包一层安全中间件：Host 校验 + API token 校验。
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			http.Error(w, "forbidden: bad host", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && !s.checkToken(r) {
			http.Error(w, "forbidden: missing or invalid token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackHost 校验 Host 头（host[:port]）是否回环地址。
// 兼容带/不带端口、IPv4、IPv6（方括号 [::1]:port 与裸 ::1 两种形态——
// 裸 IPv6 有多个冒号，不能按"最后一个冒号"剥端口）。
func isLoopbackHost(host string) bool {
	h := host
	if i := strings.LastIndexByte(h, ']'); i >= 0 {
		h = h[:i+1] // [::1]:port → [::1]
	} else if strings.Count(h, ":") == 1 {
		h = h[:strings.LastIndexByte(h, ':')] // host:port → host（仅 IPv4/主机名形态）
	}
	h = strings.Trim(h, "[]")
	switch h {
	case "127.0.0.1", "localhost", "::1", "0:0:0:0:0:0:0:1":
		return true
	}
	return false
}

// checkToken 校验请求携带的 token（Header 优先，其次 ?token= 查询参数），
// 常量时间比较防时序侧信道（本地工具，成本低）。
func (s *Server) checkToken(r *http.Request) bool {
	if s.Token == "" {
		return true // 未配置鉴权（嵌入式/测试用）；cmd/seren 永远生成非空 token
	}
	tok := r.Header.Get(tokenHeader)
	if tok == "" {
		tok = r.URL.Query().Get("token")
	}
	if tok == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(tok), []byte(s.Token)) == 1
}
