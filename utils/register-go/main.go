package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	apiBase          = "https://localhost"
	adminUser        = "admin"
	adminPass        = "change-me"
	machinesYAMLPath = "provision/machines.yml"
	rewriteLocalhost = true
	gatewayName      = "host.docker.internal"
	skipTLSVerify    = true
)

type Machine struct {
	Name     string `yaml:"name" json:"name"`
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	User     string `yaml:"user" json:"user"`
	Password string `yaml:"password" json:"password"`
}

func main() {
	machines, err := loadMachines(machinesYAMLPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read %s: %v\n", machinesYAMLPath, err)
		os.Exit(1)
	}
	if len(machines) == 0 {
		fmt.Println("No machines to register.")
		return
	}

	cli := httpClient()

	token, err := login(cli, adminUser, adminPass)
	if err != nil || token == "" {
		fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
		os.Exit(2)
	}

	var anyFailed bool
	for _, m := range machines {
		// minimal validation
		if m.Name == "" || m.Host == "" || m.Port == 0 || m.User == "" || m.Password == "" {
			fmt.Fprintf(os.Stderr, "Skipping incomplete entry: %+v\n", m)
			continue
		}
		// rewrite localhost -> gateway
		if rewriteLocalhost && (m.Host == "localhost" || m.Host == "127.0.0.1") {
			m.Host = gatewayName
		}
		if err := postMachine(cli, token, m); err != nil {
			anyFailed = true
			fmt.Fprintf(os.Stderr, "Failed to add %s: %v\n", m.Name, err)
		} else {
			fmt.Printf("Added %s (%s:%d)\n", m.Name, m.Host, m.Port)
		}
	}

	if anyFailed {
		os.Exit(2)
	}
}

func httpClient() *http.Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLSVerify}, //nolint:gosec
		Proxy:           http.ProxyFromEnvironment,
	}
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: tr,
	}
}

func login(cli *http.Client, user, pass string) (string, error) {
	url := apiBase + "/auth/login"
	body := map[string]string{"username": user, "password": pass}
	data, _ := json.Marshal(body)

	resp, err := cli.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", errors.New("no access_token in response")
	}
	return out.AccessToken, nil
}

func postMachine(cli *http.Client, token string, m Machine) error {
	url := apiBase + "/machines"
	b, _ := json.Marshal(m)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func loadMachines(path string) ([]Machine, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Accept: a list or {machines: [...]}
	var data any
	if err := yaml.Unmarshal(b, &data); err != nil {
		return nil, err
	}

	var items []any
	switch v := data.(type) {
	case []any:
		items = v
	case map[string]any:
		if mv, ok := v["machines"]; ok {
			if arr, ok := mv.([]any); ok {
				items = arr
			}
		}
	}

	var out []Machine
	for _, it := range items {
		mm, ok := it.(map[string]any)
		if !ok {
			continue
		}
		m := Machine{
			Name:     strings.TrimSpace(fmt.Sprint(mm["name"])),
			Host:     strings.TrimSpace(fmt.Sprint(mm["host"])),
			User:     strings.TrimSpace(fmt.Sprint(mm["user"])),
			Password: strings.TrimSpace(fmt.Sprint(mm["password"])),
		}
		// port can be number/float/string
		switch pv := mm["port"].(type) {
		case int:
			m.Port = pv
		case int64:
			m.Port = int(pv)
		case float64:
			m.Port = int(pv)
		default:
			fmt.Sscanf(strings.TrimSpace(fmt.Sprint(mm["port"])), "%d", &m.Port)
		}
		out = append(out, m)
	}
	return out, nil
}
