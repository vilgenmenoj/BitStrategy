package network

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Candle struct {
	StampHuman string  `json:"timestampHuman"`
	StampUnix  uint64  `json:"timestamp"`
	Open       float32 `json:"open"`
	High       float32 `json:"high"`
	Low        float32 `json:"low"`
	Close      float32 `json:"close"`
	Volume     float32 `json:"volume"`
}

const NetworkIndicatorKey = `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbWFpbCI6ImxhbWJkYWZpZWxkQGdtYWlsLmNvbSIsImlhdCI6MTYyNzkwMzEyNSwiZXhwIjo3OTM1MTAzMTI1fQ.lH2WNUaOUBEkbOJKuBoZoiKlgBc8aD5u1alfzEQLAdY`

const NetworkIndicatorURL = "https://api.taapi.io/"

func NetworkIndicatorRequest(url string) (jsonText string) {
	response, err := http.Get(url)

	if err != nil {
		log.Println(err)
		return "network request failure"
	}

	defer response.Body.Close()

	responseBody, err := ioutil.ReadAll(response.Body)

	if err != nil {
		log.Fatal(err)
	}

	jsonText = string(responseBody)

	return
}

func NetworkCandleRequest(granularity string) (candlePointer *Candle) {
	url := NetworkIndicatorURL + "candles?" + "secret=" + NetworkIndicatorKey + "&exchange=binance&symbol=BTC/USDT&interval=" + granularity

	jsonText := NetworkIndicatorRequest(url)

	var candleArray []Candle
	err := json.Unmarshal([]byte(jsonText), &candleArray)

	if err != nil {
		log.Println(jsonText)
		return nil
	}

	candlePointer = &candleArray[len(candleArray)-2] // the last elements shows current candle

	return
}

func NetworkWait(scd int) {
	time.Sleep(time.Duration(scd) * time.Second)
}

func NetworkPriceRequest() (price float64) {

	OkexURL := "https://www.okex.com/api/index/v3/BTC-USD/constituents"

	jsonText := NetworkIndicatorRequest(OkexURL)

	if jsonText == "network request failure" {
		log.Println(jsonText)
		return -1.0
	}

	index1 := strings.Index(jsonText, "last") + 7

	jsonText1 := jsonText[index1:]
	index2 := strings.Index(jsonText1, "\"")

	priceText := jsonText1[0:index2]

	price, err := strconv.ParseFloat(priceText, 64)

	if err != nil {
		log.Fatal(err)
	}

	return
}
