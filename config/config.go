// Config loads configuration.
package config

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

const Version = "1.2"

// GetInt loads the environment variable varName, converts it to an integer,
// and returns that integer or an error.
func GetInt(varName string) (int, error) {
	envVar := os.Getenv(varName)
	return strconv.Atoi(envVar)
}

func GetURLOrBail(urlEnvVar string) *url.URL {
	downstreamUrl := os.Getenv(urlEnvVar)
	if downstreamUrl == "" {
		log.Fatal(fmt.Errorf("config: no downstream URL configured. please set %s", urlEnvVar))
	}
	parsedUrl, err := url.Parse(downstreamUrl)
	if err != nil {
		log.Fatalf("config: invalid downstream url: %s. %s\n", downstreamUrl, err.Error())
	}
	return parsedUrl
}

// SetMaxIdleConnsPerHost sets the MaxIdleConnsPerHost value for the default
// HTTP transport. If you are using a custom transport, calling this function
// won't change anything.
func SetMaxIdleConnsPerHost(maxConns int) {
	// TODO: this is very brittle!!!
	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = maxConns
}
