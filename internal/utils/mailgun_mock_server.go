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
	httpServer      *httptest.Server
	apiToken        string
	activeDomains   []string
	activeMXDomains []string
	failedDomains   []string
	domainList      []mailgun.DomainContainer
	mutex           sync.Mutex
}

const (
	unknownDNS      string = "unknown"
	validDNS        string = "valid"
	unverifiedState string = "unverified"
	activeState     string = "active"
)

func NewMailgunServer(apiToken string) *MailgunMockServer {
	s := &MailgunMockServer{
		apiToken: apiToken,
	}
	s.domainList = []mailgun.DomainContainer{}
	s.activeDomains = []string{}
	s.Start()
	return s
}

func (m *MailgunMockServer) Start() {
	mux := http.NewServeMux()
	m.initRoutes(mux)
	m.httpServer = httptest.NewServer(mux)
}

func (m *MailgunMockServer) URL() string {
	return m.httpServer.URL + "/v4"
}

func (m *MailgunMockServer) Stop() {
	m.httpServer.Close()
}

func (m *MailgunMockServer) AddDomain(domainName string) {
	defer m.mutex.Unlock()
	m.mutex.Lock()
	newDomain := m.newMGDomainFor(domainName)

	receiveRecords := m.newMGDnsRecordsFor(domainName, false)
	sendingRecords := m.newMGDnsRecordsFor(domainName, true)

	m.domainList = append(m.domainList, mailgun.DomainContainer{
		Domain:              newDomain,
		ReceivingDNSRecords: receiveRecords,
		SendingDNSRecords:   sendingRecords,
	})
}

func (m *MailgunMockServer) ActivateDomain(domainName string) {
	defer m.mutex.Unlock()
	m.mutex.Lock()

	m.activeDomains = append(m.activeDomains, domainName)
}

func (m *MailgunMockServer) ActivateMXDomain(domainName string) {
	defer m.mutex.Unlock()
	m.mutex.Lock()

	m.activeMXDomains = append(m.activeMXDomains, domainName)
}

func (m *MailgunMockServer) FailedDomain(domainName string) {
	defer m.mutex.Unlock()
	m.mutex.Lock()

	m.failedDomains = append(m.failedDomains, domainName)
}

func (m *MailgunMockServer) DeleteDomain(domainName string) {
	defer m.mutex.Unlock()
	m.mutex.Lock()
	for i, domain := range m.domainList {
		if domain.Domain.Name == domainName {
			m.domainList = slices.Delete(m.domainList, i, i+1)
			break
		}
	}
}

// Private methods

func (m *MailgunMockServer) initRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v4/domains", m.authHandler(m.createDomain))
	mux.HandleFunc("GET /v4/domains", m.authHandler(m.getDomains))
	mux.HandleFunc("GET /v4/domains/{domain}", m.authHandler(m.getDomain))
	mux.HandleFunc("PUT /v4/domains/{domain}/verify", m.authHandler(m.verifyDomain))
	mux.HandleFunc("DELETE /v4/domains/{domain}", m.authHandler(m.deleteDomain))
}

func (m *MailgunMockServer) createDomain(w http.ResponseWriter, r *http.Request) {
	defer m.mutex.Unlock()
	m.mutex.Lock()

	domainName := r.FormValue("name")

	if slices.Contains(m.failedDomains, domainName) {
		toJSON(w, map[string]string{
			"message": "Failed to create domain due to invalid DNS configuration.",
		}, http.StatusInternalServerError)
		return
	}

	newDomain := m.newMGDomainFor(domainName)

	receiveRecords := m.newMGDnsRecordsFor(domainName, false)
	sendingRecords := m.newMGDnsRecordsFor(domainName, true)

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
	defer m.mutex.Unlock()
	m.mutex.Lock()
	domainName := r.PathValue("domain")
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

func (m *MailgunMockServer) verifyDomain(w http.ResponseWriter, r *http.Request) {
	defer m.mutex.Unlock()
	m.mutex.Lock()
	domainName := r.PathValue("domain")
	activateDomain := false
	if slices.Contains(m.activeDomains, domainName) {
		activateDomain = true
	}

	for i, domain := range m.domainList {
		if domainName == domain.Domain.Name {
			if activateDomain {
				m.domainList[i].Domain.State = activeState
			}
			for j := range domain.ReceivingDNSRecords {
				m.domainList[i].ReceivingDNSRecords[j].Valid = validDNS
			}
			for j := range domain.SendingDNSRecords {
				m.domainList[i].SendingDNSRecords[j].Valid = validDNS
			}
			toJSON(w, map[string]interface{}{
				"message":               "Domain DNS records have been updated",
				"domain":                m.domainList[i].Domain,
				"receiving_dns_records": m.domainList[i].ReceivingDNSRecords,
				"sending_dns_records":   m.domainList[i].SendingDNSRecords,
			}, http.StatusOK)
			return
		}
	}
	toJSON(w, map[string]interface{}{
		"message": "Domain not found",
	}, http.StatusNotFound)
}

func (m *MailgunMockServer) deleteDomain(w http.ResponseWriter, r *http.Request) {
	defer m.mutex.Unlock()
	m.mutex.Lock()
	domainName := r.PathValue("domain")

	for i, domain := range m.domainList {
		if domainName == domain.Domain.Name {
			m.domainList = slices.Delete(m.domainList, i, i+1)
			toJSON(w, map[string]interface{}{
				"message": "Domain have been removed",
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
	w.Header().Set("Content-Type", "application/json")
	b, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(status)
	_, err = w.Write(b)
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
}

func (m *MailgunMockServer) newMGDomainFor(domainName string) mailgun.Domain {
	activeDomain := unverifiedState

	if slices.Contains(m.activeDomains, domainName) {
		activeDomain = activeState
	}

	return mailgun.Domain{
		CreatedAt:  mailgun.RFC2822Time(time.Now().UTC()),
		Name:       domainName,
		SMTPLogin:  "postmaster@" + domainName,
		Wildcard:   true,
		SpamAction: mailgun.SpamActionDisabled,
		State:      activeDomain,
		WebScheme:  "http",
	}
}

func (m *MailgunMockServer) newMGDnsRecordsFor(domainName string, sendingRecords bool) []mailgun.DNSRecord {
	var records []mailgun.DNSRecord

	dnsValid := unknownDNS
	mxDnsValid := unknownDNS

	if slices.Contains(m.activeDomains, domainName) {
		dnsValid = validDNS
	}

	if slices.Contains(m.activeMXDomains, domainName) && sendingRecords {
		mxDnsValid = validDNS
	}

	if sendingRecords {
		records = []mailgun.DNSRecord{
			{
				RecordType: "TXT",
				Valid:      dnsValid,
				Name:       domainName,
				Value:      "v=spf1 include:mailgun.org ~all",
			},
			{
				RecordType: "TXT",
				Valid:      dnsValid,
				Name:       "d.mail." + domainName,
				Value:      "k=rsa; p=MIGfMA0GCSqGSIb3DQEBAQUA....",
			},
			{
				RecordType: "CNAME",
				Valid:      dnsValid,
				Name:       "email." + domainName,
				Value:      "mailgun.org",
			},
		}
	} else {
		records = []mailgun.DNSRecord{
			{
				Priority:   "10",
				RecordType: "MX",
				Valid:      mxDnsValid,
				Value:      "mxa.mailgun.org",
			},
			{
				Priority:   "10",
				RecordType: "MX",
				Valid:      mxDnsValid,
				Value:      "mxb.mailgun.org",
			},
		}
	}

	return records
}
