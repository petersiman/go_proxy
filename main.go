package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"regexp"
	"fmt"
	"flag"
	"os"
)

// Hop-by-hop headers. These are removed when sent to the backend.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te", // canonicalized version of "TE"
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func delHopHeaders(header http.Header) {
	for _, h := range hopHeaders {
		header.Del(h)
	}
}

func appendHostToXForwardHeader(header http.Header, host string) {
	// If we aren't the first proxy retain prior
	// X-Forwarded-For information as a comma+space
	// separated list and fold multiple headers into one.
	if prior, ok := header["X-Forwarded-For"]; ok {
		host = strings.Join(prior, ", ") + ", " + host
	}
	header.Set("X-Forwarded-For", host)
}

type proxy struct {
}

func (p *proxy) ServeHTTP(wr http.ResponseWriter, req *http.Request) {
	var headers string
	for k,v := range req.Header {
		headers =  headers + "," + k + " : " + strings.Join(v, ",")
	}
	log.Print(req.RemoteAddr, " ", req.Method, " ", req.URL, " Headers: ", headers)

	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
	  	msg := "unsupported protocal scheme "+req.URL.Scheme
		http.Error(wr, msg, http.StatusBadRequest)
		log.Println(msg)
		return
	}

	//Return 404 if Auth header
	if req.Header.Get("WWW-Authenticate") != "" || req.Header.Get("Authorization") != "" || req.Header.Get("WWW-Authenticate") != "" || req.Header.Get("Proxy-Authorization") != "" {
		http.Error(wr, "", http.StatusNotFound)
		log.Println("found authentication header")
		return
	}

	if req.Method == "HEAD" {
		wr.WriteHeader(http.StatusOK)
		log.Println("HEAD request - returning OK")
		return
	}

	//Return 404 if CONNECT
	if req.Method == "CONNECT" {
		http.Error(wr, "", http.StatusNotFound)
		log.Println("CONNECT request - returning 404")
		return
	}

	match, _ := regexp.MatchString("(http(s?):)([/|.|\\w|\\s|-])*\\.(?:jpg|gif|png)", req.URL.String())

	if match {
		wr.WriteHeader(http.StatusOK)
		log.Println("Image request")
		return
	}

	wr.WriteHeader(http.StatusOK)
	emptyHtml := "<html></html>"
	fmt.Fprint(wr, emptyHtml)
	log.Println("Returning empty HTML")
	return

	// Actual proxying - should never happen --->
	client := &http.Client{}

	//http: Request.RequestURI can't be set in client requests.
	//http://golang.org/src/pkg/net/http/client.go
	req.RequestURI = ""

	delHopHeaders(req.Header)

	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		appendHostToXForwardHeader(req.Header, clientIP)
	}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(wr, "Server Error", http.StatusInternalServerError)
		log.Fatal("ServeHTTP:", err)
	}
	defer resp.Body.Close()

	log.Println(req.RemoteAddr, " ", resp.Status)

	delHopHeaders(resp.Header)

	copyHeader(wr.Header(), resp.Header)
	wr.WriteHeader(resp.StatusCode)
	io.Copy(wr, resp.Body)
}

func openLogFile(path string) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Logging to a file %s \n", file.Name())

	defer file.Close()

	log.SetOutput(file)
}

func main() {
	path := flag.String("logPath", "", "Log file path (Required)")
	flag.Parse()

	if *path == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	openLogFile(*path)
	addr := ":8181"

	handler := &proxy{}

	log.Print("Starting proxy server on", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
