/*
An example of how to use go-icap.

Run this program and Squid on the same machine.
Put the following lines in squid.conf:

icap_enable on
icap_service service_req reqmod_precache icap://127.0.0.1:1344/filter
adaptation_access service_req allow all

(The ICAP server needs to be started before Squid is.)

Set your browser to use the Squid proxy.
*/
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"github.com/go-icap/icap"
	"os"
	"strconv"
	"strings"
)

var ISTag = "\"SB\""
var debug *string
var port *string
var block_page *string
var defaultAnswer *string
var apiHttpUrl *string

var err error

func check_tcp(host string, port string) string{
	if *debug != "no" {
		fmt.Fprintln(os.Stderr, "ERRlog: reporting query => \"" + host +":" + port + "\"")
		fmt.Fprintln(os.Stderr, "ERRlog: reporting db query url => \"" + *apiHttpUrl + "/tcp/?host=" + host + "&" + "port=" + port + "\"")
	}
	client := &http.Client{}
	request, err := http.NewRequest("GET", *apiHttpUrl + "/tcp/?host=" + host + "&" + "port=" + port, nil)
	request.Close = true

	resp, err := client.Do(request)
	if err != nil {
		if *debug != "no" { 
			fmt.Fprintln(os.Stderr, "ERRlog: reporting a http connection error1 => \"" + err.Error() + "\"")
		}
		return "DUNO"
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		if *debug != "no" {
			fmt.Fprintln(os.Stderr, "ERRlog: reporting a http connection error2 => \"" + err.Error() + "\"")
		}
		return "DUNO"
	}

	if body != nil && len(body) > 1{
		return string(body)
	}
	return "DUNO"
}

func check(uri string) string{
	encstr := url.QueryEscape(uri)
	if *debug != "no" {
		fmt.Fprintln(os.Stderr, "ERRlog: reporting query => \"" + uri + "\"")
		fmt.Fprintln(os.Stderr, "ERRlog: reporting db query url => \"" + *apiHttpUrl + "/url/?url=" + encstr + "\"")
	}

	client := &http.Client{}
	request, err := http.NewRequest("GET", *apiHttpUrl + "/url/?url=" + encstr, nil)
	request.Close = true

	resp, err := client.Do(request)
	if err != nil {
		if *debug != "no" {
			fmt.Fprintln(os.Stderr, "ERRlog: reporting a http connection error => \"" + err.Error() + "\"")
		}
		return "DUNO"
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		if *debug != "no" {
			fmt.Fprintln(os.Stderr, "ERRlog: reporting body read error => \"" + err.Error() + "\"")
		}
	}
	if body != nil && len(body) > 1{
		return string(body)
	}
	return "DUNO"
}

func filterByUrl(w icap.ResponseWriter, req *icap.Request) {
	h := w.Header()
	h.Set("ISTag", ISTag)
	h.Set("Service", "SquidBlocker ICAP service")

	if *debug == "Yes" {
		fmt.Fprintln(os.Stderr, "Printing the full ICAP request")
		fmt.Fprintln(os.Stderr, req)
		fmt.Fprintln(os.Stderr, req.Request)
	}
	switch req.Method {
		case "OPTIONS":
			h.Set("Methods", "REQMOD, RESPMOD")
			h.Set("Options-TTL", "1800")
			h.Set("Allow", "204")
			h.Set("Preview", "0")
			h.Set("Transfer-Preview", "*")
			h.Set("Max-Connections", "4000")
			h.Set("X-Include", "X-Client-IP, X-Authenticated-Groups, X-Authenticated-User, X-Subscriber-Id")
			w.WriteHeader(200, nil, false)
		case "REQMOD":
			if *debug == "Yes" {
				fmt.Fprintln(os.Stderr, "/filter REQMOD")
			}
			checkhost := ""
			port := "0"
			answer := *defaultAnswer
			var err error
			if *debug == "Yes" {
				fmt.Fprintln(os.Stderr, "Default CASE. Request to host: " + req.Request.URL.Host + ", Request Method: " + req.Request.Method)
				fmt.Fprintln(os.Stderr, "The full url from the ICAP client request: " + req.Request.URL.String())
			}	

			checkhost, port, err = net.SplitHostPort(req.Request.URL.Host)
			if err != nil {
				_ = err
				checkhost = req.Request.URL.Host
			}	
	
			if port != "0" {
				if *debug == "Yes" {
					fmt.Fprintln(os.Stderr, "Rquest with port: " + port)
				}
			}
			
			if req.Request.Method == "CONNECT" && len(checkhost) > 0 && port != "0" {
				answer = check_tcp(checkhost, port)
			} else {
				answer = check(req.Request.URL.String())
			}
			
			if *debug == "Yes" {
				fmt.Fprintln(os.Stderr, "ERRlog: reporting answer size => " + strconv.Itoa(len(answer)))
				fmt.Fprintln(os.Stderr, "ERRlog: reporitng answer => " + answer + ", for =>" + req.Request.URL.String())
			}

			// The next part comes to make sure that a DUNO respnse will be handled using the default answer
			if strings.HasPrefix(answer, "DUNO") {
				answer = *defaultAnswer + " rate=100 default_answer=yes"
				if *debug == "Yes" {
					fmt.Fprintln(os.Stderr,"ERRlog: reporting answer startsWith => \"DUNO\", taking default action")
					if len(*defaultAnswer) > 0 {
						fmt.Fprintln(os.Stderr, req.Request.Host + " " + *defaultAnswer + " rate=40 default_answer=yes")
					} else {
						fmt.Fprintln(os.Stderr, req.Request.Host + " OK state=DUNO")
					}
				}
			}
			
			if strings.HasPrefix(answer, "OK") {
				if *debug == "Yes" {
					fmt.Fprintln(os.Stderr, "OK response and sending 204 back")
				}
				w.WriteHeader(204, nil, false)
				return
			} 
			if strings.HasPrefix(answer, "ERR") {
				fmt.Fprintln(os.Stderr, "ERR response and sending 307 back")
				resp := new(http.Response)
				resp.Status = "307 SquidBlocker url has been filtered!"
				resp.StatusCode = 307
				resp.Proto = "HTTP/1.1"
				resp.ProtoMajor = 1
				resp.ProtoMinor = 1
				myMap := make(map[string][]string)
				//if it is a connect request
				myMap["Location"] = append(myMap["Location"], *block_page + "?url=" + url.QueryEscape(req.Request.URL.String()))			
				resp.Header = myMap
				//resp.Body = ioutil.NopCloser(bytes.NewBufferString(body))
				//resp.ContentLength = int64(len(body))
				resp.Request = req.Request
				w.WriteHeader(200, resp, true)
				return
			}
			if *debug == "Yes" {
					fmt.Fprintln(os.Stderr, "Unknown asnwer and scenario, not adapting the request")
			}
			w.WriteHeader(204, nil, false)
			return
		case "RESPMOD":
			w.WriteHeader(204, nil, false)
		default:
			w.WriteHeader(405, nil, false)
			if *debug == "Yes" {
				fmt.Fprintln(os.Stderr,"Invalid request method")
			}
	}
}

func defaultIcap(w icap.ResponseWriter, req *icap.Request) {
	h := w.Header()
	h.Set("ISTag", ISTag)
	h.Set("Service", "SquidBlocker ICAP service")

	if *debug == "Yes" {
		fmt.Fprintln(os.Stderr, "Printing the full ICAP request")
		fmt.Fprintln(os.Stderr, req)
		fmt.Fprintln(os.Stderr, req.Request)
	}
	switch req.Method {
		case "OPTIONS":
			h.Set("Methods", "REQMOD, RESPMOD")
			h.Set("Options-TTL", "1800")
			h.Set("Allow", "204")
			h.Set("Preview", "0")
			h.Set("Transfer-Preview", "*")
			h.Set("Max-Connections", "4000")
			h.Set("This-Server", "Defualt ICAP url which bypass all requests adaptation")
			h.Set("X-Include", "X-Client-IP, X-Authenticated-Groups, X-Authenticated-User, X-Subscriber-Id")
			w.WriteHeader(200, nil, false)
		case "REQMOD":
			if *debug == "Yes" {
				fmt.Fprintln(os.Stderr, "Default REQMOD, you should use the apropriate ICAP URL")
			}
			w.WriteHeader(204, nil, false)	
		case "RESPMOD":
			if *debug == "Yes" {
				fmt.Fprintln(os.Stderr, "Default RESPMOD, you should use the apropriate ICAP URL")
			}
			w.WriteHeader(204, nil, false)
		default:
			w.WriteHeader(405, nil, false)
			if *debug == "Yes" {
				fmt.Fprintln(os.Stderr, "Invalid request method")
			}
	}
}

func init() {
	fmt.Fprintln(os.Stderr, "ERRlog: Starting SquidBlocker ICAP service")

	debug = flag.String("d", "no", "Debug mode can be \"Yes\" or something else for no")
	port = flag.String("p", "1344", "Listening port for the ICAP service")
	block_page = flag.String("b", "http://ngtech.co.il/block_page/", "A url which will be used as a block page with the domains/host appended")
	apiHttpUrl = flag.String("u", "http://filterdb:8080/sb/01", "Db base path")
	defaultAnswer = flag.String("a", "OK", "Answer can be either \"ERR\" or \"OK\"")
	flag.Parse()
}

func main() {
	fmt.Fprintln(os.Stderr, "running SquidBlocker ICAP serivce :D")

	if *debug == "Yes" {
		fmt.Fprintln(os.Stderr, "Config Variables:")
		fmt.Fprintln(os.Stderr, "Debug: => " + *debug)
		fmt.Fprintln(os.Stderr, "api http url: => " + *apiHttpUrl)
		fmt.Fprintln(os.Stderr, "Defualt Answer: => " + *defaultAnswer)
	}
	icap.HandleFunc("/filter", filterByUrl)
	icap.HandleFunc("/", defaultIcap)
	icap.ListenAndServe(":"+*port, nil)
}
