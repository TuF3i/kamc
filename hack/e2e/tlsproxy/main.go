// 本地 TLS 终结器（联调专用）：把 TLS 连接原样转发到平台监听地址。
//
// 用途：kubectl/client-go 不会向 http:// 端点发送 Bearer Token，本地验证
// 必须先有 https。生产环境由 Nginx 提供 TLS，本工具只在开发机使用：
//
//	go run ./hack/e2e/tlsproxy -listen 127.0.0.1:8443 -upstream 127.0.0.1:8080 \
//	    -cert /tmp/kamc-tls/cert.pem -key /tmp/kamc-tls/key.pem
package main

import (
	"crypto/tls"
	"flag"
	"io"
	"log"
	"net"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8443", "TLS 监听地址")
	upstream := flag.String("upstream", "127.0.0.1:8080", "平台明文监听地址")
	certFile := flag.String("cert", "/tmp/kamc-tls/cert.pem", "证书文件")
	keyFile := flag.String("key", "/tmp/kamc-tls/key.pem", "私钥文件")
	flag.Parse()

	cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
	if err != nil {
		log.Fatal(err)
	}
	ln, err := tls.Listen("tcp", *listen, &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("[tlsproxy] %s -> %s", *listen, *upstream)
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go func(conn net.Conn) {
			defer conn.Close()
			up, err := net.Dial("tcp", *upstream)
			if err != nil {
				return
			}
			defer up.Close()
			go io.Copy(up, conn)
			_, _ = io.Copy(conn, up)
		}(conn)
	}
}
