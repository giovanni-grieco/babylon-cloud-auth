package registry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

// Client registers and deregisters this service instance with Consul.
type Client struct {
	consulAddr  string
	consulPort  string
	instanceID  string
	serviceName string
	servicePort int
	httpClient  *http.Client
}

func New(consulAddr, consulPort, serviceName string, servicePort int) *Client {
	return &Client{
		consulAddr:  consulAddr,
		consulPort:  consulPort,
		instanceID:  hostname(),
		serviceName: serviceName,
		servicePort: servicePort,
		httpClient:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *Client) Register() bool {
	ip := containerIP()
	payload := serviceRegistration{
		ID:      c.instanceID,
		Name:    c.serviceName,
		Address: ip,
		Port:    c.servicePort,
		Tags:    []string{"traefik.enable=true"},
		Check: healthCheck{
			HTTP:                           fmt.Sprintf("http://%s:%d/health", ip, c.servicePort),
			Interval:                       "10s",
			Timeout:                        "2s",
			DeregisterCriticalServiceAfter: "30s",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return false
	}

	req, err := http.NewRequest(http.MethodPut, c.agentURL()+"/service/register", bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("register: unexpected status %d", resp.StatusCode)
		return false
	}

	log.Printf("registered with Consul as %s (%s)", c.instanceID, payload.Address)
	return true
}

func (c *Client) Deregister() {
	req, err := http.NewRequest(http.MethodPut, c.agentURL()+"/service/deregister/"+c.instanceID, nil)
	if err != nil {
		return
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("deregistration failed: %v", err)
		return
	}
	defer resp.Body.Close()
	log.Printf("deregistered %s from Consul", c.instanceID)
}

func (c *Client) RegisterLoop() {
	for !c.Register() {
		time.Sleep(5 * time.Second)
	}
}

func (c *Client) agentURL() string {
	return fmt.Sprintf("http://%s:%s/v1/agent", c.consulAddr, c.consulPort)
}

type healthCheck struct {
	HTTP                           string `json:"HTTP"`
	Interval                       string `json:"Interval"`
	Timeout                        string `json:"Timeout"`
	DeregisterCriticalServiceAfter string `json:"DeregisterCriticalServiceAfter"`
}

type serviceRegistration struct {
	ID      string      `json:"ID"`
	Name    string      `json:"Name"`
	Address string      `json:"Address"`
	Port    int         `json:"Port"`
	Tags    []string    `json:"Tags"`
	Check   healthCheck `json:"Check"`
}

// containerIP returns this container's IP on the docker network.
func containerIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return "127.0.0.1"
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return fmt.Sprintf("auth-%d", time.Now().UnixNano())
	}
	return name
}
