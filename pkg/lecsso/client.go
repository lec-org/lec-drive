// Package lecsso 实现 LecSSO 的标准 OIDC 登录，身份以 issuer 和 subject 共同标识。
package lecsso

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Config struct {
	Issuer, ClientID, ClientSecret, Origin, Organization string
	AdminSubjects                                        []string
}

func Enabled() bool { return os.Getenv("LEC_SSO_ISSUER") != "" }

func ReadConfig() (Config, error) {
	c := Config{
		Issuer:   strings.TrimSuffix(os.Getenv("LEC_SSO_ISSUER"), "/"),
		ClientID: os.Getenv("LEC_SSO_CLIENT_ID"), ClientSecret: os.Getenv("LEC_SSO_CLIENT_SECRET"),
		Origin:        strings.TrimSuffix(os.Getenv("LEC_DRIVE_ORIGIN"), "/"),
		Organization:  os.Getenv("LEC_SSO_ORGANIZATION"),
		AdminSubjects: strings.FieldsFunc(os.Getenv("LEC_DRIVE_ADMIN_SUBJECTS"), func(r rune) bool { return r == ',' }),
	}
	if c.ClientID == "" || c.ClientSecret == "" || c.Origin == "" || c.Issuer == "" {
		return c, errors.New("请完整配置 LecSSO 登录")
	}
	issuer, err := url.Parse(c.Issuer)
	if err != nil || issuer.Host == "" || issuer.User != nil || issuer.RawQuery != "" || issuer.Fragment != "" {
		return c, errors.New("LecSSO issuer 无效")
	}
	origin, err := url.Parse(c.Origin)
	if err != nil || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return c, errors.New("LecDrive 来源必须是完整站点地址")
	}
	local := func(u *url.URL) bool {
		return u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"
	}
	// 明文仅用于显式开启的本机 SSH 隧道；生产环境和远程域名始终要求 HTTPS。
	allowLocal := os.Getenv("LEC_SSO_ALLOW_INSECURE_LOCALHOST") == "1" && os.Getenv("NODE_ENV") != "production" && local(issuer) && local(origin)
	if issuer.Scheme != "https" && !(issuer.Scheme == "http" && allowLocal) {
		return c, errors.New("LecSSO 必须使用 HTTPS")
	}
	if origin.Scheme != "https" && !(origin.Scheme == "http" && allowLocal) {
		return c, errors.New("LecDrive 必须使用 HTTPS")
	}
	return c, nil
}

type Claims struct {
	Subject       string   `json:"sub"`
	Name          string   `json:"name"`
	Username      string   `json:"username"`
	Picture       string   `json:"picture"`
	Organizations []string `json:"organizations"`
}

type Client struct {
	config   Config
	provider *oidc.Provider
	oauth    oauth2.Config
	http     *http.Client
}

func New(ctx context.Context, config Config) (*Client, error) {
	httpClient := &http.Client{Timeout: 15 * time.Second}
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, httpClient), config.Issuer)
	if err != nil {
		return nil, fmt.Errorf("LecSSO 暂不可用: %w", err)
	}
	return &Client{config: config, provider: provider, http: httpClient, oauth: oauth2.Config{
		ClientID: config.ClientID, ClientSecret: config.ClientSecret,
		Endpoint: provider.Endpoint(), RedirectURL: config.Origin + "/session/lec-sso/callback",
		Scopes: []string{oidc.ScopeOpenID, "profile", "urn:logto:scope:organizations"},
	}}, nil
}

func (c *Client) LoginURL(state, nonce, verifier string) string {
	return c.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
}

func (c *Client) Complete(ctx context.Context, code, nonce, verifier string) (*Claims, error) {
	ctx = oidc.ClientContext(ctx, c.http)
	tokens, err := c.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, errors.New("LecSSO 授权码兑换失败，请重新登录")
	}
	raw, ok := tokens.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("LecSSO 未返回身份凭证")
	}
	token, err := c.provider.Verifier(&oidc.Config{ClientID: c.config.ClientID}).Verify(ctx, raw)
	if err != nil {
		return nil, errors.New("LecSSO 身份凭证校验失败")
	}
	if token.Nonce != nonce {
		return nil, errors.New("LecSSO 登录随机数不匹配")
	}
	var claims Claims
	if err := token.Claims(&claims); err != nil || claims.Subject == "" {
		return nil, errors.New("LecSSO 用户身份无效")
	}
	if c.config.Organization != "" && !slices.Contains(claims.Organizations, c.config.Organization) {
		return nil, errors.New("当前账号尚未加入 LEC 工作室")
	}
	return &claims, nil
}
