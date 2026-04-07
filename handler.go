package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
)

func handleRequest(w http.ResponseWriter, r *http.Request) {
	log.Printf("Incoming request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	ipStr, err := getLookupIP(r)
	if err != nil {
		log.Printf("Invalid lookup request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	info := lookupIP(ipStr)
	format := responseFormat(r)

	switch format {
	case "json":
		log.Printf("Serving JSON response for IP: %s", info.IP)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(info); err != nil {
			log.Printf("JSON encoding error: %v", err)
		}
	case "html":
		log.Printf("Serving HTML response for IP: %s", info.IP)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, info); err != nil {
			log.Printf("Template execution error: %v", err)
		}
	default:
		log.Printf("Serving plain text response for IP: %s", info.IP)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "IP: %s\n", info.IP)
		if info.CIDR != "" {
			fmt.Fprintf(w, "CIDR: %s\n", info.CIDR)
		}
		if info.ASN != 0 {
			fmt.Fprintf(w, "ASN: AS%d\n", info.ASN)
		}
		if info.Name != "" {
			fmt.Fprintf(w, "Name: %s\n", info.Name)
		}
		if info.Org != "" {
			fmt.Fprintf(w, "Org: %s\n", info.Org)
		}
		if info.CountryCode != "" {
			fmt.Fprintf(w, "Country: %s\n", info.CountryCode)
		}
		if info.Domain != "" {
			fmt.Fprintf(w, "Domain: %s\n", info.Domain)
		}
	}
}

func responseFormat(r *http.Request) string {
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	switch format {
	case "json", "html", "text":
		return format
	}

	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "application/json") {
		return "json"
	}
	if strings.Contains(accept, "text/html") {
		return "html"
	}

	return "text"
}

func getLookupIP(r *http.Request) (string, error) {
	if r.URL.Path == "/" {
		queryIP := r.URL.Query().Get("ip")
		if queryIP != "" {
			parsedIP := net.ParseIP(queryIP)
			if parsedIP == nil {
				return "", fmt.Errorf("invalid IP address")
			}
			log.Printf("Using IP from query parameter: %s", parsedIP.String())
			return parsedIP.String(), nil
		}
		clientIP := getClientIP(r)
		log.Printf("Using client IP: %s", clientIP)
		return clientIP, nil
	}

	pathIP := strings.TrimPrefix(r.URL.Path, "/")
	if pathIP == "" || strings.Contains(pathIP, "/") {
		return "", fmt.Errorf("invalid path")
	}

	parsedIP := net.ParseIP(pathIP)
	if parsedIP == nil {
		return "", fmt.Errorf("invalid IP address")
	}

	log.Printf("Using IP from URL path: %s", parsedIP.String())
	return parsedIP.String(), nil
}

func getClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
