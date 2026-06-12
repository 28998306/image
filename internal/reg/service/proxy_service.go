package service

import (
	"context"
	"fmt"
	"net/url"

	"web2img/internal/reg/crypto"
	"web2img/internal/reg/model"
	"web2img/internal/reg/repo"
)

// ProxyService 代理服务：列举启用代理 + 把行装配成可用 URL。
type ProxyService struct {
	repo *repo.ProxyRepo
	aes  *crypto.AESGCM
}

// NewProxyService 构造。
func NewProxyService(r *repo.ProxyRepo, aes *crypto.AESGCM) *ProxyService {
	return &ProxyService{repo: r, aes: aes}
}

// ListEnabled 全部启用代理。
func (s *ProxyService) ListEnabled(ctx context.Context) ([]*model.Proxy, error) {
	return s.repo.ListEnabled(ctx)
}

// BuildURL 把代理行装配成 scheme://user:pass@host:port。
func (s *ProxyService) BuildURL(p *model.Proxy) (*url.URL, error) {
	if p == nil {
		return nil, nil
	}
	scheme := p.Protocol
	if scheme == "" {
		scheme = model.ProxyProtoHTTP
	}
	u := &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", p.Host, p.Port),
	}
	if p.Username != nil && *p.Username != "" {
		pass := ""
		if len(p.PasswordEnc) > 0 && s.aes != nil {
			if dec, err := s.aes.Decrypt(p.PasswordEnc); err == nil {
				pass = string(dec)
			} else {
				return nil, err
			}
		}
		u.User = url.UserPassword(*p.Username, pass)
	}
	return u, nil
}
