package lecsso

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestOIDC验证签名与登录绑定(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	for _, scenario := range []struct {
		name                          string
		audience, nonce, organization string
		expired, valid                bool
	}{
		{name: "正常登录", audience: "drive", nonce: "nonce", organization: "lec-studio", valid: true},
		{name: "错误客户端", audience: "other", nonce: "nonce", organization: "lec-studio"},
		{name: "错误随机数", audience: "drive", nonce: "other", organization: "lec-studio"},
		{name: "非工作室成员", audience: "drive", nonce: "nonce", organization: "other"},
		{name: "过期凭证", audience: "drive", nonce: "nonce", organization: "lec-studio", expired: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			verifier := oauth2.GenerateVerifier()
			var issuer string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/.well-known/openid-configuration":
					json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"}})
				case "/jwks":
					json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "test", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
				case "/token":
					r.ParseForm()
					if r.Form.Get("code_verifier") != verifier {
						w.WriteHeader(400)
						return
					}
					expiry := time.Now().Add(time.Minute)
					if scenario.expired {
						expiry = time.Now().Add(-time.Minute)
					}
					token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": issuer, "aud": scenario.audience, "sub": "member", "exp": expiry.Unix(), "iat": time.Now().Unix(), "nonce": scenario.nonce, "organizations": []string{scenario.organization}})
					token.Header["kid"] = "test"
					signed, err := token.SignedString(key)
					if err != nil {
						t.Error(err)
						w.WriteHeader(500)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"access_token": "test", "token_type": "Bearer", "id_token": signed})
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			issuer = server.URL
			client, err := New(context.Background(), Config{Issuer: issuer, ClientID: "drive", ClientSecret: "secret", Origin: "http://127.0.0.1:5212", Organization: "lec-studio"})
			require.NoError(t, err)
			login, err := url.Parse(client.LoginURL("state", "nonce", verifier))
			require.NoError(t, err)
			require.Equal(t, "S256", login.Query().Get("code_challenge_method"))
			require.NotEmpty(t, login.Query().Get("code_challenge"))
			require.Equal(t, "state", login.Query().Get("state"))
			claims, err := client.Complete(context.Background(), "code", "nonce", verifier)
			if scenario.valid {
				require.NoError(t, err)
				require.Equal(t, "member", claims.Subject)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func Test仅显式允许本机隧道使用HTTP(t *testing.T) {
	t.Setenv("LEC_SSO_CLIENT_ID", "drive")
	t.Setenv("LEC_SSO_CLIENT_SECRET", "secret")
	t.Setenv("LEC_DRIVE_ORIGIN", "http://127.0.0.1:5212")
	t.Setenv("LEC_SSO_ISSUER", "http://127.0.0.1:33101/oidc")
	t.Setenv("LEC_SSO_ALLOW_INSECURE_LOCALHOST", "")
	_, err := ReadConfig()
	require.Error(t, err)
	t.Setenv("LEC_SSO_ALLOW_INSECURE_LOCALHOST", "1")
	_, err = ReadConfig()
	require.NoError(t, err)
	t.Setenv("NODE_ENV", "production")
	_, err = ReadConfig()
	require.Error(t, err)
	t.Setenv("NODE_ENV", "development")
	t.Setenv("LEC_SSO_ISSUER", "http://sso.example.org/oidc")
	_, err = ReadConfig()
	require.Error(t, err)
}
