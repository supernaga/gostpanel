package gost

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"github.com/AliceNetworks/gost-panel/internal/model"
)

// GenerateProxyURI 生成代理 URI
func GenerateProxyURI(node *model.Node) string {
	switch node.Protocol {
	case "socks5":
		return generateSocks5URI(node)
	case "socks4":
		return generateSocks4URI(node)
	case "http":
		return generateHTTPURI(node)
	case "ss":
		return generateShadowsocksURI(node)
	default:
		return generateSocks5URI(node)
	}
}

func generateSocks5URI(node *model.Node) string {
	// socks5://user:pass@host:port
	uri := "socks5://"
	if node.ProxyUser != "" && node.ProxyPass != "" {
		uri += url.QueryEscape(node.ProxyUser) + ":" + url.QueryEscape(node.ProxyPass) + "@"
	}
	uri += fmt.Sprintf("%s:%d", node.Host, node.Port)
	return uri
}

func generateSocks4URI(node *model.Node) string {
	// socks4://host:port
	return fmt.Sprintf("socks4://%s:%d", node.Host, node.Port)
}

func generateHTTPURI(node *model.Node) string {
	// http://user:pass@host:port
	uri := "http://"
	if node.ProxyUser != "" && node.ProxyPass != "" {
		uri += url.QueryEscape(node.ProxyUser) + ":" + url.QueryEscape(node.ProxyPass) + "@"
	}
	uri += fmt.Sprintf("%s:%d", node.Host, node.Port)
	return uri
}

func generateShadowsocksURI(node *model.Node) string {
	// ss://base64(method:password)@host:port#name
	method := node.SSMethod
	if method == "" {
		method = "aes-256-gcm"
	}
	userinfo := fmt.Sprintf("%s:%s", method, node.SSPassword)
	encoded := base64.URLEncoding.EncodeToString([]byte(userinfo))
	return fmt.Sprintf("ss://%s@%s:%d#%s", encoded, node.Host, node.Port, url.QueryEscape(node.Name))
}
