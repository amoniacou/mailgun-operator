package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"time"

	"github.com/mailgun/mailgun-go/v4"
)

type MailgunMockServer struct {
	httpServer    *httptest.Server
	apiToken      string
	activeDomains []string
	domainList    []mailgun.DomainContainer
	mutex         sync.Mutex
}

func NewMailgunServer(apiToken string) *MailgunMockServer {
	return &MailgunMockServer{
		apiToken: apiToken,
	}
}

func (m *MailgunMockServer) Start() {
	mux := http.NewServeMux()
	m.initRoutes(mux)
	m.httpServer = httptest.NewServer(mux)
}

func (m *MailgunMockServer) initRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v4/domains", m.authHandler(m.createDomain))
	mux.HandleFunc("GET /v4/domains", m.authHandler(m.getDomains))
	mux.HandleFunc("GET /v4/domains/{domain}", m.authHandler(m.getDomain))
}

func (m *MailgunMockServer) createDomain(w http.ResponseWriter, r *http.Request) {
	defer m.mutex.Unlock()
	m.mutex.Lock()

	domainName := r.FormValue("name")

	activeDomain := "unverified"
	dnsValid := "unknown"

	if slices.Contains(m.activeDomains, domainName) {
		activeDomain = "active"
		dnsValid = "valid"
	}

	newDomain := mailgun.Domain{
		CreatedAt:    mailgun.RFC2822Time(time.Now().UTC()),
		Name:         domainName,
		SMTPLogin:    "postmaster@mailgun.test",
		SMTPPassword: "smtp_password",
		Wildcard:     true,
		SpamAction:   mailgun.SpamActionDisabled,
		State:        activeDomain,
		WebScheme:    "http",
	}
	receiveRecords := []mailgun.DNSRecord{
		{
			Priority:   "10",
			RecordType: "MX",
			Valid:      dnsValid,
			Value:      "mxa.mailgun.org",
		},
		{
			Priority:   "10",
			RecordType: "MX",
			Valid:      dnsValid,
			Value:      "mxb.mailgun.org",
		},
	}

	sendingRecords := []mailgun.DNSRecord{
		{
			RecordType: "TXT",
			Valid:      dnsValid,
			Name:       domainName,
			Value:      "v=spf1 include:mailgun.org ~all",
		},
		{
			RecordType: "TXT",
			Valid:      dnsValid,
			Name:       "d_" + domainName,
			Value:      "k=rsa; p=MIGfMA0GCSqGSIb3DQEBAQUA....",
		},
		{
			RecordType: "CNAME",
			Valid:      "valid",
			Name:       "email." + domainName,
			Value:      "mailgun.org",
		},
	}

	m.domainList = append(m.domainList, mailgun.DomainContainer{
		Domain:              newDomain,
		ReceivingDNSRecords: receiveRecords,
		SendingDNSRecords:   sendingRecords,
	})
	toJSON(w, map[string]interface{}{
		"message":               "Domain DNS records have been created",
		"domain":                newDomain,
		"receiving_dns_records": receiveRecords,
		"sending_dns_records":   sendingRecords,
	}, http.StatusOK)
}

func (m *MailgunMockServer) getDomains(w http.ResponseWriter, r *http.Request) {
	toJSON(w, m.domainList, http.StatusOK)
}

func (m *MailgunMockServer) getDomain(w http.ResponseWriter, r *http.Request) {
	domainName := r.URL.Query()["domain"][0]
	for _, domain := range m.domainList {
		if domainName == domain.Domain.Name {
			toJSON(w, map[string]interface{}{
				"message": "Domain DNS records have been retrieved",
				"domain":  domain,
			}, http.StatusOK)
			return
		}
	}
	toJSON(w, map[string]interface{}{
		"message": "Domain not found",
	}, http.StatusNotFound)
}

func (m *MailgunMockServer) authHandler(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, password, ok := r.BasicAuth()
		if ok {
			if password == m.apiToken {
				next.ServeHTTP(w, r)
				return
			}
		}
		toJSON(w, map[string]string{
			"message": "Invalid private key",
		}, http.StatusUnauthorized)
	})
}

func toJSON(w http.ResponseWriter, obj interface{}, status int) {
	if err := json.NewEncoder(w).Encode(obj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
}

func (m *MailgunMockServer) Stop() {
	m.httpServer.Close()
}
