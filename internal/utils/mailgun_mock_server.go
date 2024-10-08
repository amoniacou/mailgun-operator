package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/mailgun/mailgun-go/v4"
	"k8s.io/apimachinery/pkg/util/rand"
)

type MailgunMockServer struct {
	httpServer      *httptest.Server
	apiToken        string
	activeDomains   []string
	activeMXDomains []string
	failedDomains   []string
	domainList      []mailgun.DomainContainer
	routes          map[string]mailgun.Route
	failRoutes      map[string][]string
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

	receiveRecords := m.newMGDnsRecordsFor(domainName, false, slices.Contains(m.activeDomains, domainName), slices.Contains(m.activeMXDomains, domainName))
	sendingRecords := m.newMGDnsRecordsFor(domainName, true, slices.Contains(m.activeDomains, domainName), slices.Contains(m.activeMXDomains, domainName))

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

func (m *MailgunMockServer) FailRoutes(action, id string) {
	defer m.mutex.Unlock()
	m.mutex.Lock()

	if m.failRoutes == nil {
		m.failRoutes = map[string][]string{}
	}
	if _, ok := m.failRoutes[action]; !ok {
		m.failRoutes[action] = []string{}
	}
	m.failRoutes[action] = append(m.failRoutes[action], id)
}

// Private methods

func (m *MailgunMockServer) initRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v4/domains", m.authHandler(m.createDomain))
	mux.HandleFunc("GET /v4/domains", m.authHandler(m.getDomains))
	mux.HandleFunc("GET /v4/domains/{domain}", m.authHandler(m.getDomain))
	mux.HandleFunc("PUT /v4/domains/{domain}/verify", m.authHandler(m.verifyDomain))
	mux.HandleFunc("DELETE /v4/domains/{domain}", m.authHandler(m.deleteDomain))
	mux.HandleFunc("POST /v4/routes", m.authHandler(m.createRoute))
	mux.HandleFunc("GET /v4/routes/{id}", m.authHandler(m.getRoute))
	mux.HandleFunc("PUT /v4/routes/{id}", m.authHandler(m.updateRoute))
	mux.HandleFunc("DELETE /v4/routes/{id}", m.authHandler(m.deleteRoute))
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

	receiveRecords := m.newMGDnsRecordsFor(
		domainName,
		false,
		slices.Contains(m.activeDomains, domainName),
		slices.Contains(m.activeMXDomains, domainName),
	)
	sendingRecords := m.newMGDnsRecordsFor(
		domainName,
		true,
		slices.Contains(m.activeDomains, domainName),
		slices.Contains(m.activeMXDomains, domainName),
	)

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
	for i, domain := range m.domainList {
		if domainName == domain.Domain.Name {
			m.domainList[i].ReceivingDNSRecords = m.newMGDnsRecordsFor(
				domainName,
				false,
				slices.Contains(m.activeDomains, domainName),
				slices.Contains(m.activeMXDomains, domainName),
			)
			toJSON(w, map[string]interface{}{
				"message":               "Domain DNS records have been retrieved",
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

	if slices.Contains(m.failedDomains, domainName) {
		toJSON(w, map[string]string{
			"message": "Failed to create domain due to invalid DNS configuration.",
		}, http.StatusInternalServerError)
		return
	}

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

func (m *MailgunMockServer) createRoute(w http.ResponseWriter, r *http.Request) {
	defer m.mutex.Unlock()
	m.mutex.Lock()

	description := r.FormValue("description")
	if description == "fail" {
		toJSON(w, map[string]string{
			"message": "Failed to create route due to invalid expression.",
		}, http.StatusInternalServerError)
		return
	}
	expression := r.FormValue("expression")
	priority := r.FormValue("priority")
	priorityVal := 0
	if len(priority) > 0 {
		p, err := strconv.Atoi(priority)
		if err != nil {
			toJSON(w, map[string]interface{}{
				"message": "Invalid priority value",
			}, http.StatusBadRequest)
			return
		}
		priorityVal = p
	}

	id := "route-" + rand.String(10)

	newRoute := mailgun.Route{
		Id:          id,
		Priority:    priorityVal,
		Actions:     r.Form["action"],
		Description: description,
		Expression:  expression,
	}

	if m.routes == nil {
		m.routes = map[string]mailgun.Route{}
	}
	m.routes[newRoute.Id] = newRoute

	toJSON(w, map[string]interface{}{
		"message": "Route has been created",
		"route":   newRoute,
	}, http.StatusOK)
}

func (m *MailgunMockServer) getRoute(w http.ResponseWriter, r *http.Request) {
	defer m.mutex.Unlock()
	m.mutex.Lock()

	routeID := r.PathValue("id")
	if routeID == "" {
		toJSON(w, map[string]interface{}{
			"message": "Route ID is required",
		}, http.StatusBadRequest)
		return
	}

	if _, ok := m.failRoutes["get"]; ok {
		if slices.Contains(m.failRoutes["get"], routeID) {
			toJSON(w, map[string]interface{}{
				"message": "Failed to get route",
			}, http.StatusInternalServerError)
			return
		}
	}

	route, ok := m.routes[routeID]
	if !ok {
		toJSON(w, map[string]interface{}{
			"message": "Route not found",
		}, http.StatusNotFound)
		return
	}
	toJSON(w, map[string]interface{}{
		"route": route,
	}, http.StatusOK)
}

func (m *MailgunMockServer) deleteRoute(w http.ResponseWriter, r *http.Request) {
	defer m.mutex.Unlock()
	m.mutex.Lock()

	routeID := r.PathValue("id")
	if routeID == "" {
		toJSON(w, map[string]interface{}{
			"message": "Route ID is required",
		}, http.StatusBadRequest)
		return
	}

	if _, ok := m.failRoutes["delete"]; ok {
		if slices.Contains(m.failRoutes["delete"], routeID) {
			toJSON(w, map[string]interface{}{
				"message": "Failed to delete route",
			}, http.StatusInternalServerError)
			return
		}
	}

	delete(m.routes, routeID)
	toJSON(w, map[string]interface{}{
		"message": "Route has been deleted",
		"id":      routeID,
	}, http.StatusOK)
}

func (m *MailgunMockServer) updateRoute(w http.ResponseWriter, r *http.Request) {
	defer m.mutex.Unlock()
	m.mutex.Lock()

	routeID := r.PathValue("id")
	if routeID == "" {
		toJSON(w, map[string]interface{}{
			"message": "Route ID is required",
		}, http.StatusBadRequest)
		return
	}
	route, ok := m.routes[routeID]
	if !ok {
		toJSON(w, map[string]interface{}{
			"message": "Route not found",
		}, http.StatusNotFound)
		return
	}

	if r.FormValue("action") != "" {
		route.Actions = r.Form["action"]
	}
	if r.FormValue("priority") != "" {
		p, err := strconv.Atoi(r.FormValue("priority"))
		if err != nil {
			toJSON(w, map[string]interface{}{
				"message": "Invalid priority value",
			}, http.StatusBadRequest)
			return
		}
		route.Priority = p
	}
	if r.FormValue("description") != "" {
		route.Description = r.FormValue("description")
	}
	if r.FormValue("expression") != "" {
		route.Expression = r.FormValue("expression")
	}

	m.routes[routeID] = route
	toJSON(w, map[string]interface{}{
		"message": "Route has been updated",
		"route":   route,
	}, http.StatusOK)
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

func (m *MailgunMockServer) newMGDnsRecordsFor(domainName string, sendingRecords, activeDomain, activeMXRecord bool) []mailgun.DNSRecord {
	var records []mailgun.DNSRecord

	dnsValid := unknownDNS
	mxDnsValid := unknownDNS

	if activeDomain {
		dnsValid = validDNS
	}

	if activeMXRecord {
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
