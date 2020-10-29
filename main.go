package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-go/statsd"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"time"
)

func check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

type metricSeries struct {
	MetricName string     `json:"metric"`
	Points     [][]string `json:"points"`
	Tags       []string   `json:"tags"`
	MetricType string     `json:"type"`
}

type metricData struct {
	Series []metricSeries `json:"series"`
}

func main() {
	datadogAPIKey := os.Getenv("DD_API_KEY")
	datadogAPIUrl := fmt.Sprintf("https://api.datadoghq.com/api/v1/series?api_key=%s", datadogAPIKey)
	statsdHost := "127.0.0.1:8125"
	if os.Getenv("STATSTD_HOST") != "" {
		statsdHost = os.Getenv("STATSTD_HOST")
	}
	devicesDir := "/sys/bus/w1/devices/"
	if os.Getenv("DEVICES_DIR") != "" {
		devicesDir = os.Getenv("DEVICES_DIR")
	}

	statsd, err := statsd.New(statsdHost)
	check(err)

	var devices []string

	files, err := ioutil.ReadDir(devicesDir)
	check(err)

	deviceRegexp := regexp.MustCompile(`^28.*`)

	for _, f := range files {
		matched := deviceRegexp.MatchString(f.Name())
		if f.IsDir() && matched {
			devices = append(devices, f.Name())
		}
	}
	log.Printf("devices found: %q", devices)

	temperatureRegexp := regexp.MustCompile(`(?s)^.*t\=(\d+)\n$`)

	pollInterval := int64(30)
	if os.Getenv("POLL_INTERVAL") != "" {
		pollInterval, err = strconv.ParseInt(os.Getenv("POLL_INTERVAL"), 10, 32)
		check(err)
	}

	for {
		for _, device := range devices {
			deviceFile := path.Join(devicesDir, device, "w1_slave")
			dat, err := ioutil.ReadFile(deviceFile)
			check(err)
			// log.Printf("device: %s\ncontents:\n%s\n", device, string(dat))
			temperatureCelciusMatch := temperatureRegexp.FindSubmatch(dat)
			// log.Printf("%q", temperatureCelciusMatch)
			if temperatureCelciusMatch == nil {
				log.Fatalf("could not parse temperature from file: %s\ncontents: %s", deviceFile, string(dat))
			}
			temperatureCelcius, err := strconv.ParseFloat(string(temperatureCelciusMatch[1]), 32)
			temperatureCelcius = temperatureCelcius / 1000
			check(err)

			log.Printf("device: %s, temperature (celcius): %f", device, temperatureCelcius)
			tags := []string{fmt.Sprintf("device:%s", device)}

			// statsd
			_ = statsd.Gauge("w1_temperature.celcius.gauge", temperatureCelcius, tags, 1)
			// http
			metricPoints := []string{fmt.Sprintf("%v", time.Now().Unix()), fmt.Sprintf("%f", temperatureCelcius)}
			fmt.Println(time.Now().Unix())

			var series []metricSeries
			series = append(series, metricSeries{MetricName: "w1_temperature.celcius.gauge", Points: [][]string{metricPoints}, Tags: tags, MetricType: "gauge"})
			//  jsonSeries, err := json.Marshal(series)
			payload := metricData{Series: series}
			jsonPayload, err := json.Marshal(payload)
			check(err)
			log.Printf("JSON payload: %s\n", jsonPayload)
			check(err)
			req, err := http.NewRequest("POST", datadogAPIUrl, bytes.NewBuffer(jsonPayload))
			check(err)
			req.Header.Set("Content-Type", "application/json")
			client := &http.Client{}
			resp, err := client.Do(req)
			check(err)
			defer resp.Body.Close()
			log.Println("response Status:", resp.Status)
			body, _ := ioutil.ReadAll(resp.Body)
			log.Println("response Body:", string(body))
		}
		time.Sleep(time.Duration(pollInterval) * time.Second)
	}
}
