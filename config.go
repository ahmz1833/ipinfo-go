package main

import (
	"html/template"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

const (
	dbURL      = "https://github.com/iplocate/ip-address-databases/raw/main/ip-to-asn/ip-to-asn.mmdb"
	dbDir      = "/var/cache/ipinfo"
	dbPath     = dbDir + "/ip-to-asn.mmdb"
	updateFreq = 24 * time.Hour
	listenAddr = ":8080"

	downloadConnections = 12
	progressInterval    = time.Second
	minDBSize           = 10 * 1024 * 1024
)

type IPInfo struct {
	IP          string `json:"ip"`
	CIDR        string `json:"cidr,omitempty"`
	ASN         uint   `json:"asn,omitempty"`
	Name        string `json:"name,omitempty"`
	Org         string `json:"org,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	Domain      string `json:"domain,omitempty"`
	LookupOK    bool   `json:"-"`
	LookupState string `json:"-"`
}

var (
	db   *maxminddb.Reader
	dbMu sync.RWMutex
	tmpl *template.Template
)
