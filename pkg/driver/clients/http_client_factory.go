package clients

import (
	"github.com/sirupsen/logrus"
	"net"
	"net/http"
	"time"
)

func CreateHTTPClient(timeout int, invokeProtocol string) *http.Client {
	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	switch invokeProtocol {
	case "http1":
		client.Transport = getHttp1Transport(timeout)
	case "http2":
		client.Transport = getHttp2Transport()
	case "grpc":
	default:
		logrus.Errorf("Invalid invoke protocol in the configuration file.")
	}

	return client
}

func getHttp1Transport(timeout int) *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: time.Duration(timeout) * time.Second,
		}).DialContext,
		IdleConnTimeout:     5 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     10,
	}
}

func getHttp2Transport() *http.Transport {
	// Cleartext HTTP/2 with prior knowledge (h2c)
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)

	return &http.Transport{
		Protocols: protocols,
	}
}
