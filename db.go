package main

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

func bootstrapDatabase() (bool, error) {
	if fileInfo, err := os.Stat(dbPath); err == nil && fileInfo.Size() > minDBSize {
		log.Printf("Existing database found: %s (size: %d bytes)", dbPath, fileInfo.Size())
		if err := loadDB(); err != nil {
			log.Printf("Cached database failed validation: %v", err)
			if rmErr := os.Remove(dbPath); rmErr != nil {
				return true, fmt.Errorf("failed to remove invalid database %s: %w", dbPath, rmErr)
			}
			log.Println("Removed invalid cached database, performing blocking re-download...")
			if err := updateDB(); err != nil {
				return true, fmt.Errorf("recovery update failed: %w", err)
			}
		}
		return true, nil
	} else if err == nil {
		log.Printf("Existing database is corrupted (too small: %d bytes). Removing.", fileInfo.Size())
		if rmErr := os.Remove(dbPath); rmErr != nil {
			return false, fmt.Errorf("failed to remove corrupted database: %w", rmErr)
		}
	}

	log.Println("No valid database found, performing blocking download...")
	if err := updateDB(); err != nil {
		return false, err
	}

	return false, nil
}

func startUpdateTicker() {
	log.Printf("Starting database update ticker (frequency: %s)", updateFreq)
	ticker := time.NewTicker(updateFreq)
	for range ticker.C {
		if err := updateDB(); err != nil {
			log.Printf("Scheduled database update failed: %v", err)
		}
	}
}

func updateDB() error {
	log.Println("Starting database update process")
	if err := downloadDB(); err != nil {
		log.Printf("Database download failed: %v", err)
		return err
	}
	return loadDB()
}

func downloadDB() error {
	log.Printf("Creating temporary directory for download in %s", dbDir)
	tmpDir, err := os.MkdirTemp(dbDir, "dl-*")
	if err != nil {
		return err
	}
	defer func() {
		log.Printf("Cleaning up temporary directory: %s", tmpDir)
		os.RemoveAll(tmpDir)
	}()

	parsedURL, err := url.Parse(dbURL)
	if err != nil {
		return fmt.Errorf("invalid dbURL: %w", err)
	}
	expectedFile := path.Base(parsedURL.Path)
	downloadedFile := filepath.Join(tmpDir, expectedFile)

	log.Printf("Starting download from %s", dbURL)
	if err := downloadFileWithProgress(dbURL, downloadedFile); err != nil {
		return err
	}
	log.Printf("Download completed: %s", downloadedFile)

	if _, err := os.Stat(downloadedFile); err != nil {
		return fmt.Errorf("expected downloaded file %q not found in temp dir: %w", expectedFile, err)
	}

	if err := validateDownloadedDB(downloadedFile); err != nil {
		return err
	}

	log.Printf("Download successful. Moving %s to %s", downloadedFile, dbPath)
	return os.Rename(downloadedFile, dbPath)
}

func validateDownloadedDB(downloadedFile string) error {
	info, err := os.Stat(downloadedFile)
	if err != nil {
		return err
	}

	if info.Size() < minDBSize {
		content, _ := os.ReadFile(downloadedFile)
		preview := string(content)
		if len(preview) > 100 {
			preview = preview[:100]
		}
		return fmt.Errorf("downloaded file %s is too small (%d bytes), likely an LFS pointer. Preview: %s", filepath.Base(downloadedFile), info.Size(), preview)
	}

	testDB, err := maxminddb.Open(downloadedFile)
	if err != nil {
		return fmt.Errorf("downloaded file validation failed: %w", err)
	}
	defer testDB.Close()

	if err := validateDBIntegrity(testDB); err != nil {
		return fmt.Errorf("downloaded file integrity validation failed: %w", err)
	}

	return nil
}

func loadDB() error {
	log.Printf("Loading database into memory from: %s", dbPath)
	dbMu.Lock()
	defer dbMu.Unlock()

	if fileInfo, err := os.Stat(dbPath); err == nil {
		log.Printf("Database file size: %d bytes", fileInfo.Size())
	}

	if db != nil {
		log.Println("Closing previous database instance")
		db.Close()
	}

	newDB, err := maxminddb.Open(dbPath)
	if err != nil {
		log.Printf("Failed to open database: %v", err)
		return err
	}

	if err := validateDBIntegrity(newDB); err != nil {
		newDB.Close()
		log.Printf("Database integrity validation failed: %v", err)
		return err
	}

	db = newDB
	log.Println("Database loaded into memory successfully")
	return nil
}

func validateDBIntegrity(r *maxminddb.Reader) error {
	testIP := net.ParseIP("1.1.1.1")
	if testIP == nil {
		return fmt.Errorf("failed to parse integrity-check IP")
	}

	var record map[string]interface{}
	if _, _, err := r.LookupNetwork(testIP, &record); err != nil {
		return fmt.Errorf("integrity lookup failed: %w", err)
	}

	return nil
}

func lookupIP(ipStr string) IPInfo {
	info := IPInfo{IP: ipStr, LookupState: "Lookup failed"}
	parsedIP := net.ParseIP(ipStr)
	if parsedIP == nil {
		log.Printf("Lookup failed: invalid IP format for %s", ipStr)
		return info
	}

	dbMu.RLock()
	defer dbMu.RUnlock()

	if db == nil {
		log.Println("Lookup failed: database is not initialized")
		return info
	}

	var record map[string]interface{}
	network, ok, err := db.LookupNetwork(parsedIP, &record)
	if err != nil {
		log.Printf("Database lookup error for %s: %v", ipStr, err)
		return info
	}
	if !ok {
		log.Printf("Lookup returned no record for: %s", ipStr)
		return info
	}

	info.LookupOK = true
	info.LookupState = "Lookup complete"

	if network != nil {
		info.CIDR = network.String()
	}
	if record != nil {
		if asn, ok := record["asn"]; ok {
			info.ASN = toUint(asn)
		}
		if name, ok := record["name"].(string); ok {
			info.Name = name
		}
		if org, ok := record["org"].(string); ok {
			info.Org = org
		}
		if cc, ok := record["country_code"].(string); ok {
			info.CountryCode = cc
		}
		if domain, ok := record["domain"].(string); ok {
			info.Domain = domain
		}
	}

	if info.ASN != 0 {
		log.Printf("Lookup successful: %s -> AS%d (%s, %s, %s)", info.IP, info.ASN, info.Name, info.CountryCode, info.CIDR)
	} else {
		log.Printf("Lookup returned no ASN data for: %s", info.IP)
	}

	return info
}

func toUint(v interface{}) uint {
	switch val := v.(type) {
	case uint:
		return val
	case uint16:
		return uint(val)
	case uint32:
		return uint(val)
	case uint64:
		return uint(val)
	case int:
		return uint(val)
	case int32:
		return uint(val)
	case int64:
		return uint(val)
	case float64:
		return uint(val)
	case string:
		val = strings.TrimSpace(strings.ToUpper(val))
		val = strings.TrimPrefix(val, "AS")
		var u uint
		fmt.Sscanf(val, "%d", &u)
		return u
	}
	return 0
}
