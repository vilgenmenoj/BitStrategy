package strategy

import (
	json "encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	data "../data"
	network "../network"
)

type Bollinger struct {
	Upper     float32 `json:"valueUpperBand"`
	Middle    float32 `json:"valueMiddleBand"`
	Lower     float32 `json:"valueLowerBand"`
	Backtrack uint16  `json:"backtrack"`
}

type BollingerStateMachine struct {
	ControlState    uint8
	ExecuteState    uint8
	OpeningPosition float64
	ClosingPosition float64
	Lever           float64
}

type PriceSlot struct {
	Price               float64
	AboveBollingerLower bool
}

const (
	STATE_INACTIVE uint8 = 0
	STATE_PREPARE  uint8 = 1
	STATE_HOLDON   uint8 = 2
	STATE_EXE      uint8 = 3

	EXE_INACTIVE      uint8 = 0
	EXE_ACTIVE        uint8 = 1
	EXE_ACTIVE_MIDDLE uint8 = 2
	EXE_EXPIRE        uint8 = 9
	EXE_BAD_TRADE     uint8 = 10
	EXE_GOOD_TRADE    uint8 = 20
	EXE_NEUTRAL_TRADE uint8 = 30

	BollingerIndictor = "bbands"
)

var OpenStamp int64 = 0

func BollingerCalculateLever(c *network.Candle, b *Bollinger) (lever float64) {
	if c.High >= (b.Middle+b.Lower)/2 {
		lever = 1
	} else {
		lever = 5
	}

	return
}

func BollingerClosingTrade(state *BollingerStateMachine, price float64) {
	state.ClosingPosition = price
	log.Printf("[executor] close trading %+v\n", state)

	//hang the trade
	state.ExecuteState = EXE_EXPIRE
	state.ClosingPosition = 0
	state.OpeningPosition = 0
	state.Lever = 0
}

func BollingerLightenUpTrade(price float64) {
	log.Printf("[executor] lighten up %+f%% at price %+f\n", 0.333*100.0, price)
}

func BollingerOpeningTrade() (openStamp int64) {
	openStamp = time.Now().Unix()
	log.Printf("[controller] record opening trade time %d", openStamp)
	return
}

//check if the price is below bollinger lower band within a duration of time
func PriceUnderBollingerDuration(thres int, series *[]PriceSlot) (breakBollinger bool) {

	if thres > len(*series) {
		return false
	}

	for t := len(*series) - 1; t >= len(*series)-thres; t-- {
		if (*series)[t].AboveBollingerLower {
			return false
		}
	}

	return true
}

func BollingerRequest(isRealTime uint8) (c *network.Candle, b *Bollinger) {

	var parameter string = "exchange=binance&symbol=BTC/USDT&interval=1h&backtracks=3&period=20"

	finalURL := network.NetworkIndicatorURL + BollingerIndictor + "?" + "secret=" + network.NetworkIndicatorKey + "&" + parameter

	//query bollinger data
	jsonText := network.NetworkIndicatorRequest(finalURL)

	var bollingerArray []Bollinger
	err := json.Unmarshal([]byte(jsonText), &bollingerArray)

	if err != nil && isRealTime == 1 {
		log.Println(jsonText)
		return nil, nil
	}

	if isRealTime == 1 {
		b = &bollingerArray[0]
		c = nil

		return
	}

	network.NetworkWait(15)

	//query candle data
	lastCandle := network.NetworkCandleRequest("1h")
	c = lastCandle

	lastBollinger := &bollingerArray[1]
	b = lastBollinger

	if c == nil || b == nil {
		log.Fatalln("Candle or Bollinger updates failed due to network failure")
	}

	var temp1, temp2 []byte

	temp1, err = json.Marshal(lastCandle)

	if err != nil {
		log.Println(temp1)
		return nil, nil
	}

	temp2, err = json.Marshal(lastBollinger)

	if err != nil {
		log.Println(temp2)
		return nil, nil
	}

	lastBondRecord := string(temp1) + "^" + string(temp2)

	data.DataPush("data/bollinger.db", lastBondRecord)

	return
}

func BollingerController(candles *[]network.Candle, bollingers *[]Bollinger, state *BollingerStateMachine) {

	log.Println("[controller] startup")

	//trigger this every hour at _:00:01(first query), finish at _:00:17(second query). During __:59:43 to __:00:34, realtime bollinger query is forbidden
	for {
		prCandle, prBollinger := BollingerRequest(0)

		*candles = append(*candles, *prCandle)
		*bollingers = append(*bollingers, *prBollinger)

		clens := len(*candles)

		log.Printf("[controller] candle %+v\n", *prCandle)
		log.Printf("[controller] bollinger %+v\n", *prBollinger)
		log.Printf("[controller] state FROM %+v \n", *state)

		switch state.ControlState {
		case STATE_INACTIVE:
			if prCandle.Low <= prBollinger.Lower {
				state.ControlState = STATE_PREPARE
			}

		case STATE_PREPARE:
			if prCandle.Close > prCandle.Open && prCandle.High < prBollinger.Middle {
				state.ControlState = STATE_HOLDON
			} else {
				if prCandle.Low <= prBollinger.Lower {
					//do nothing, hold on
				} else {
					state.ControlState = STATE_INACTIVE
				}
			}

		case STATE_HOLDON:
			if (prCandle.Open+prCandle.Close) > ((*candles)[clens-2].Open+(*candles)[clens-2].Close) && prCandle.High < prBollinger.Middle {
				state.ControlState = STATE_EXE
				OpenStamp = BollingerOpeningTrade()
			} else {
				if prCandle.Low <= prBollinger.Lower {
					state.ControlState = STATE_PREPARE
				} else {
					state.ControlState = STATE_INACTIVE
				}
			}

		case STATE_EXE:
			if state.ExecuteState == EXE_EXPIRE {
				//closed, thread hang up
				state.ExecuteState = EXE_INACTIVE
				state.ControlState = STATE_INACTIVE
			}
		}

		log.Printf("[controller] state TO %+v \n", *state)

		var stampCurrent, residue, sleepDuration int64
		stampCurrent = time.Now().Unix()
		residue = stampCurrent - (stampCurrent/3600)*3600
		sleepDuration = 3600 - residue - 15

		//sleep one hour
		time.Sleep(time.Duration(sleepDuration) * time.Second)
	}
}

func BollingerRealTime(brt *Bollinger) {

	log.Println("[realtime bollinger monitor] startup")

	//query realtime bollinger every 16 seconds
	for {

		var stampCurrent, residue int64
		stampCurrent = time.Now().Unix()
		residue = stampCurrent - (stampCurrent/3600)*3600

		if residue > 34 && residue <= 3583 {
			_, b := BollingerRequest(1)

			if b != nil {
				brt.Lower = b.Lower
				brt.Middle = b.Middle
				brt.Upper = b.Upper
			} else {
				log.Println("bollinger request failed for one period")
			}
		}

		network.NetworkWait(16)
	}
}

func BollingerExecutor(CandleMemory *[]network.Candle, BollingerMemory *[]Bollinger, state *BollingerStateMachine, bollingerRealTime *Bollinger) {

	var priceSeries []PriceSlot
	var queryCycle int = 2

	for {
		runtime.NumGoroutine()

		var stampCurrent, residue int64
		stampCurrent = time.Now().Unix()
		residue = stampCurrent - (stampCurrent/3600)*3600

		if residue >= 14 && residue <= 20 {
			network.NetworkWait(1)
			continue
		}

		price := network.NetworkPriceRequest()

		if price < 0 {
			log.Println("price request failed for a period")
			network.NetworkWait(10)
			continue
		}

		priceSeries = append(priceSeries, PriceSlot{price, (price >= float64((*BollingerMemory)[len(*BollingerMemory)-1].Lower))})
		if len(priceSeries) >= 3600/queryCycle {
			priceSeries = priceSeries[1800/queryCycle:]
		}

		fmt.Println(priceSeries[len(priceSeries)-1])

		if state.ControlState == STATE_EXE {

			if state.ExecuteState == EXE_EXPIRE {
				network.NetworkWait(10)
				continue
			}

			if state.ExecuteState == EXE_INACTIVE {

				//tailor the price memory
				priceSeries = priceSeries[len(priceSeries)-10:]

				state.ExecuteState = EXE_ACTIVE
				state.OpeningPosition = price
				state.Lever = BollingerCalculateLever(&(*CandleMemory)[len(*CandleMemory)-1], &(*BollingerMemory)[len(*BollingerMemory)-1])
				log.Printf("[executor] open trading %+v\n", state)
			}

			if price <= float64(bollingerRealTime.Lower) {

				stampCurrent_ := time.Now().Unix()

				if stampCurrent_-OpenStamp > 0 && stampCurrent_-OpenStamp < 3000 {

					//hold on 20 minutes
					if PriceUnderBollingerDuration(60/queryCycle*20, &priceSeries) {
						log.Printf("[executor] break bollinger lower band")
						state.ExecuteState = EXE_BAD_TRADE
						BollingerClosingTrade(state, price)
					}

				} else {
					state.ExecuteState = EXE_BAD_TRADE
					BollingerClosingTrade(state, price)
				}

			} else if price >= float64(bollingerRealTime.Middle) && price < float64(bollingerRealTime.Upper) {

				if bollingerRealTime.Middle-float32(state.OpeningPosition) <= float32(state.OpeningPosition)*0.0041 {
					log.Printf("[executor] access middle but close immediately")
					state.ExecuteState = EXE_NEUTRAL_TRADE
					BollingerClosingTrade(state, price)
					network.NetworkWait(10)
					continue
				}

				if state.ExecuteState != EXE_ACTIVE_MIDDLE {
					state.ExecuteState = EXE_ACTIVE_MIDDLE
					BollingerLightenUpTrade(price)
					log.Printf("[executor] access middle %+v\n", state)
				}

			} else if price >= float64(bollingerRealTime.Upper) {
				state.ExecuteState = EXE_GOOD_TRADE //good closing
				BollingerClosingTrade(state, price)

			} else if price < float64(bollingerRealTime.Middle) && state.ExecuteState == EXE_ACTIVE_MIDDLE {

				if price <= (0.333*float64(bollingerRealTime.Middle) + 0.667*state.OpeningPosition) {
					state.ExecuteState = EXE_NEUTRAL_TRADE // neutral closing
					BollingerClosingTrade(state, price)
				}

			}

			//accerate the query frequency when activate the executor
			network.NetworkWait(queryCycle)

		} else {
			network.NetworkWait(10)
		}
	}
}

func BollingerStart() {

	lFile, err := os.OpenFile("strategy/bollinger.log", os.O_APPEND|os.O_RDWR|os.O_CREATE, 0666)

	if err != nil {
		log.Fatal(err)
	}

	log.SetOutput(lFile)
	log.SetFlags(log.Lshortfile | log.LstdFlags)

	defer lFile.Close()

	var CandleMemory []network.Candle
	var BollingerMemory []Bollinger
	var state BollingerStateMachine = BollingerStateMachine{STATE_INACTIVE, EXE_INACTIVE, 0, 0, 0}
	var bollingerRealTime Bollinger

	//startup controller thread
	go BollingerController(&CandleMemory, &BollingerMemory, &state)

	time.Sleep(35 * time.Second)

	//startup bollinger updating thread
	go BollingerRealTime(&bollingerRealTime)

	//remain as executor thread, query every 10 seconds, except from _:00:14 to _:00:20
	log.Printf("[executor] startup")
	BollingerExecutor(&CandleMemory, &BollingerMemory, &state, &bollingerRealTime)

}
