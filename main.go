package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var hosts struct {
	HostName string `json:"host_name"`
}

var tr = &http.Transport{
	TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
}
var client = &http.Client{Transport: tr}

func main() {
	i2Host := flag.String("i2Host", "localhost:5665", "Icinga2 host and port")
	i2Auth := flag.String("i2Auth", "root:icinga", "Icinga2 curl authentication")
	iw2Host := flag.String("iw2Host", "10.211.55.14:80", "Icinga Web 2 host and port")
	iw2Auth := flag.String("iw2Auth", "icingaadmin:icinga", "Icinga Web 2 authentication")
	cnfdPath := flag.String("cnfdPath", "/etc/icinga2/conf.d", "Icinga2 conf.d absolute path")
	flag.Parse()

	for {
		byteArr := make([]byte, 6)
		if _, err := rand.Read(byteArr); err != nil {
			log.Fatalf("Error while generating random file: %s", err)
		}

		data := fmt.Sprintf("%X", byteArr)
		path := CreateConfigFile(cnfdPath, data)

		credential := strings.Split(*i2Auth, ":")
		body := strings.NewReader("")
		request := UpdateObject(body, fmt.Sprintf("https://%s/v1/actions/restart-process", *i2Host))
		SetupRequestHeader(request, credential)

		log.Print("Sending API request to restart Icinga 2 process")

		resp, err := client.Do(request)
		if err != nil {
			log.Fatalf("Failed to restart Icinga 2 process: %s", err)
		}
		resp.Body.Close()

		log.Print("Waiting while Icinga 2 process is restarting")
		time.Sleep(30 * time.Second)

		log.Printf("Removing Icinga 2 %s config file", path)
		err = os.Remove(path)
		if err != nil {
			log.Fatalf("Error while deleting config file: %s", err)
		}

		log.Print("Sending API request to restart Icinga 2 process")

		resp, err = client.Do(request)
		if err != nil {
			log.Fatalf("Failed to restart Icinga 2 process: %s", err)
		}
		resp.Body.Close()

		log.Print("Waiting while Icinga 2 process is restarting")
		time.Sleep(20 * time.Second)

		// Querying Icinga Web 2 for host created using config file
		iw2Credential := strings.Split(*iw2Auth, ":")
		IW2Request(iw2Credential, data, iw2Host)

		body = strings.NewReader(`{ "attrs": {
      		"check_command": "hostalive",
      		"address": "10.211.55.14",
      		"display_name": "icingaservice",
      		"vars": { "os": "Linux"} }
    	}`)
		_ = CreateHost(body, i2Host, data, credential)

		log.Printf("Waiting until '%s' object of type Host is checked", data)
		time.Sleep(10 * time.Second)

		// Querying Icinga Web 2 for host created via API
		IW2Request(iw2Credential, data, iw2Host)
		// Querying Icinga2 for hos object host $data
		I2Request(credential, data, i2Host)

		body = strings.NewReader(`{ "attrs": {
      		"check_command": "hostalive",
      		"address": "10.211.55.14",
      		"display_name": "newicingaservice",
      		"vars": { "os": "MacOS"} }
    	}`)
		// Create a new host with new object name via the API
		_ = CreateHost(body, i2Host, "icinga2", credential)

		log.Print("Waiting until 'icinga2' object of type Host is checked")
		time.Sleep(10 * time.Second)

		// Querying Icinga Web 2 for host created via API
		IW2Request(iw2Credential, "icinga2", iw2Host)
		// Querying Icinga2 for hos object host $data
		I2Request(credential, "icinga2", i2Host)

		DeleteHost(fmt.Sprintf("https://%s/v1/objects/hosts/%s?cascade=1", *i2Host, data), data, credential)
		DeleteHost(fmt.Sprintf("https://%s/v1/objects/hosts/%s?cascade=1", *i2Host, "icinga2"), "icinga2", credential)

		log.Print("Waiting until all deleted hosts are also removed from IDO")
		time.Sleep(30 * time.Second)
	}
}

func CreateHost(body *strings.Reader, host *string, object string, credential []string) *http.Response {
	req, err := http.NewRequest("PUT", os.ExpandEnv(fmt.Sprintf("https://%s/v1/objects/hosts/%s", *host, object)), body)
	if err != nil {
		log.Fatalf("Failed to create host: %s", err)
	}

	SetupRequestHeader(req, credential)
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Request failed: %s", err)
	}
	resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Printf("Host '%s' object of type Host successfully created via API", object)
	} else {
		log.Printf("Failed to create '%s' object of type Host", object)
	}

	return resp
}

func UpdateObject(body *strings.Reader, url string) *http.Request {
	req, err := http.NewRequest("POST", os.ExpandEnv(url), body)
	if err != nil {
		log.Fatalf("Failed to update object: %s", err)
	}

	return req
}

func GetHost(body *strings.Reader, url string) *http.Request {
	req, err := http.NewRequest("GET", os.ExpandEnv(url), body)
	if err != nil {
		log.Fatalf("Error while getting objects: %s", err)
	}

	return req
}

func DeleteHost(url string, object string, credential []string) *http.Response {
	req, err := http.NewRequest("DELETE", os.ExpandEnv(url), nil)
	if err != nil {
		log.Fatalf("Error while deleting: %s", err)
	}

	SetupRequestHeader(req, credential)
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Request failed: %s", err)
	}
	resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Printf("Host '%s' object of type Host successfully deleted", object)
	} else {
		log.Printf("Failed to delete '%s' object of type Host", object)
	}

	return resp
}

func SetupRequestHeader(req *http.Request, cred []string) {
	req.SetBasicAuth(cred[0], cred[1])
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
}

func CreateConfigFile(path *string, object string) string {
	filePath, _ := filepath.Abs(fmt.Sprintf("%s/%s.conf", *path, object))
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatalf("Failed to create config file: %s", err)
	}
	defer file.Close()

	_, err = file.WriteString(
		"object Host \"" + object + "\" {\n\t" +
			"display_name = \"icingaservice\"\n\t" +
			"check_command = \"dummy\"\n\t" +
			"enable_active_checks = false\n\t" +
			"enable_notifications = \"1.000000\"\n}")
	if err != nil {
		log.Fatalf("Error while writing into %s: %s", filePath, err)
	}
	_ = file.Sync()

	log.Printf("Icinga 2 '%s' config file successfully created", filePath)

	return filePath
}

func IW2Request(credential []string, data string, host *string)  {
	body := strings.NewReader("")
	request := GetHost(body, fmt.Sprintf("http://%s/icingaweb2/monitoring/list/hosts?host=%s&modifyFilter=1", *host, data))
	SetupRequestHeader(request, credential)

	resp, err := client.Do(request)
	if err != nil {
		log.Fatalf("Failed getting request: %s", err)
	}
	resp.Body.Close()

	_ = json.NewDecoder(bufio.NewReader(resp.Body)).Decode(&hosts)

	if hosts.HostName == "" {
		log.Printf("Host '%s' object doesn't exists in Icinga Web 2", data)
	} else {
		log.Printf("Host '%s' object can be found in Icinga Web 2", hosts.HostName)
	}
}

func I2Request(credential []string, data string, host *string) {
	body := strings.NewReader("")
	request := GetHost(body, fmt.Sprintf("https://%s/v1/objects/hosts/%s?pretty=1", *host, data))
	SetupRequestHeader(request, credential)

	resp, err := client.Do(request)
	if err != nil {
		log.Fatalf("Failed getting request: %s", err)
	}
	resp.Body.Close()

	_ = json.NewDecoder(bufio.NewReader(resp.Body)).Decode(&hosts)

	if hosts.HostName == "" {
		log.Printf("Host '%s' object doesn't exists in Icinga 2", data)
	} else {
		log.Printf("Host '%s' object can be found in Icinga 2", hosts.HostName)
	}
}
