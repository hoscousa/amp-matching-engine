package services

import (
	"log"
	"math"
	"time"

	"github.com/Proofsuite/amp-matching-engine/interfaces"
	"github.com/Proofsuite/amp-matching-engine/types"
	"github.com/Proofsuite/amp-matching-engine/utils"
	"github.com/Proofsuite/amp-matching-engine/ws"
	"gopkg.in/mgo.v2/bson"

	"github.com/ethereum/go-ethereum/common"
)

type OHLCVService struct {
	tradeDao interfaces.TradeDao
}

func NewOHLCVService(TradeDao interfaces.TradeDao) *OHLCVService {
	return &OHLCVService{TradeDao}
}

// Unsubscribe handles all the unsubscription messages for ticks corresponding to a pair
func (s *OHLCVService) Unsubscribe(conn *ws.Conn, bt, qt common.Address, params *types.Params) {
	id := utils.GetOHLCVChannelID(bt, qt, params.Units, params.Duration)
	ws.GetTradeSocket().Unsubscribe(id, conn)
}

// Subscribe handles all the subscription messages for ticks corresponding to a pair
// It calls the corresponding channel's subscription method and sends trade history back on the connection
func (s *OHLCVService) Subscribe(conn *ws.Conn, bt, qt common.Address, params *types.Params) {

	socket := ws.GetOHLCVSocket()

	ohlcv, err := s.GetOHLCV([]types.PairAddresses{types.PairAddresses{BaseToken: bt, QuoteToken: qt}},
		params.Duration,
		params.Units,
		params.From,
		params.To,
	)

	if err != nil {
		socket.SendErrorMessage(conn, err.Error())
	}

	id := utils.GetOHLCVChannelID(bt, qt, params.Units, params.Duration)
	err = socket.Subscribe(id, conn)
	if err != nil {
		message := map[string]string{
			"Code":    "UNABLE_TO_SUBSCRIBE",
			"Message": "UNABLE_TO_SUBSCRIBE: " + err.Error(),
		}

		socket.SendErrorMessage(conn, message)
	}

	ws.RegisterConnectionUnsubscribeHandler(conn, socket.UnsubscribeHandler(id))
	socket.SendInitMessage(conn, ohlcv)
}

// GetOHLCV fetches OHLCV data using
// pairName: can be "" for fetching data for all pairs
// duration: in integer
// unit: sec,min,hour,day,week,month,yr
// timeInterval: 0-2 entries (0 argument: latest data,1st argument: from timestamp, 2nd argument: to timestamp)
func (s *OHLCVService) GetOHLCV(pairs []types.PairAddresses, duration int64, unit string, timeInterval ...int64) ([]*types.Tick, error) {
	match := make(bson.M)
	addFields := make(bson.M)
	resp := make([]*types.Tick, 0)

	currentTimestamp := time.Now().Unix()

	sort := bson.M{"$sort": bson.M{"createdAt": 1}}
	toDecimal := bson.M{"$addFields": bson.M{
		"priceDecimal":  bson.M{"$toDecimal": "$pricepoint"},
		"amountDecimal": bson.M{"$toDecimal": "$amount"},
	}}

	modTime, intervalInSeconds := getModTime(currentTimestamp, duration, unit)
	group, addFields := getGroupAddFieldBson("$createdAt", unit, duration)

	log.Print(duration)
	log.Print(unit)

	end := time.Unix(currentTimestamp, 0)
	start := time.Unix(modTime-intervalInSeconds, 0)

	if len(timeInterval) >= 1 {
		end = time.Unix(timeInterval[1], 0)
		start = time.Unix(timeInterval[0], 0)
	}

	match = getMatchQuery(start, end, pairs...)
	match = bson.M{"$match": match}
	group = bson.M{"$group": group}

	query := []bson.M{match, sort, toDecimal, group, addFields, {"$sort": bson.M{"ts": 1}}}

	resp, err := s.tradeDao.Aggregate(query)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func getMatchQuery(start, end time.Time, pairs ...types.PairAddresses) bson.M {
	match := bson.M{
		"createdAt": bson.M{
			"$gte": start,
			"$lt":  end,
		},
		"status": bson.M{"$in": []string{"SUCCESS"}},
	}

	if len(pairs) >= 1 {
		or := make([]bson.M, 0)

		for _, pair := range pairs {
			or = append(or, bson.M{
				"$and": []bson.M{
					{
						"baseToken":  pair.BaseToken.Hex(),
						"quoteToken": pair.QuoteToken.Hex(),
					},
				},
			},
			)
		}

		match["$or"] = or
	}
	return match
}

func getModTime(ts, interval int64, unit string) (int64, int64) {
	var modTime, intervalInSeconds int64
	switch unit {
	case "sec":
		intervalInSeconds = interval
		modTime = ts - int64(math.Mod(float64(ts), float64(intervalInSeconds)))

	case "hour":
		intervalInSeconds = interval * 60 * 60
		modTime = ts - int64(math.Mod(float64(ts), float64(intervalInSeconds)))

	case "day":
		intervalInSeconds = interval * 24 * 60 * 60
		modTime = ts - int64(math.Mod(float64(ts), float64(intervalInSeconds)))

	case "week":
		intervalInSeconds = interval * 7 * 24 * 60 * 60
		modTime = ts - int64(math.Mod(float64(ts), float64(intervalInSeconds)))

	case "month":
		d := time.Date(time.Now().Year(), time.Now().Month()+1, 1, 0, 0, 0, 0, time.UTC).Day()
		intervalInSeconds = interval * int64(d) * 24 * 60 * 60
		modTime = ts - int64(math.Mod(float64(ts), float64(intervalInSeconds)))

	case "year":
		// Number of days in current year
		d := time.Date(time.Now().Year()+1, 1, 1, 0, 0, 0, 0, time.UTC).Sub(time.Date(time.Now().Year(), 0, 0, 0, 0, 0, 0, time.UTC)).Hours() / 24
		intervalInSeconds = interval * int64(d) * 24 * 60 * 60
		modTime = ts - int64(math.Mod(float64(ts), float64(intervalInSeconds)))

	case "min":
		intervalInSeconds = interval * 60
		modTime = ts - int64(math.Mod(float64(ts), float64(intervalInSeconds)))
	}

	return modTime, intervalInSeconds
}

// query for grouping of the documents and addition of required fields using aggregate pipeline
func getGroupAddFieldBson(key, units string, duration int64) (bson.M, bson.M) {
	var group, addFields bson.M

	t := time.Unix(0, 0)
	var date interface{}
	if key == "now" {
		date = time.Now()
	} else {
		date = key
	}

	decimal1, _ := bson.ParseDecimal128("1")
	group = bson.M{
		"count": bson.M{"$sum": decimal1},
		"h":     bson.M{"$max": "$priceDecimal"},
		"l":     bson.M{"$min": "$priceDecimal"},
		"o":     bson.M{"$first": "$priceDecimal"},
		"c":     bson.M{"$last": "$priceDecimal"},
		"v":     bson.M{"$sum": "$amountDecimal"},
	}

	groupID := make(bson.M)
	switch units {
	case "sec":
		groupID = bson.M{
			"year":   bson.M{"$year": date},
			"day":    bson.M{"$dayOfMonth": date},
			"month":  bson.M{"$month": date},
			"hour":   bson.M{"$hour": date},
			"minute": bson.M{"$minute": date},
			"second": bson.M{
				"$subtract": []interface{}{
					bson.M{"$second": date},
					bson.M{"$mod": []interface{}{bson.M{"$second": date}, duration}},
				},
			},
		}

		addFields = bson.M{"$addFields": bson.M{
			"ts": bson.M{
				"$subtract": []interface{}{bson.M{
					"$dateFromParts": bson.M{
						"year":   "$_id.year",
						"month":  "$_id.month",
						"day":    "$_id.day",
						"hour":   "$_id.hour",
						"minute": "$_id.minute",
						"second": "$_id.second"}}, t}}}}

	case "min":
		groupID = bson.M{
			"year":  bson.M{"$year": date},
			"day":   bson.M{"$dayOfMonth": date},
			"month": bson.M{"$month": date},
			"hour":  bson.M{"$hour": date},
			"minute": bson.M{
				"$subtract": []interface{}{
					bson.M{"$minute": date},
					bson.M{"$mod": []interface{}{bson.M{"$minute": date}, duration}},
				}}}

		addFields = bson.M{"$addFields": bson.M{"ts": bson.M{"$subtract": []interface{}{bson.M{"$dateFromParts": bson.M{
			"year":   "$_id.year",
			"month":  "$_id.month",
			"day":    "$_id.day",
			"hour":   "$_id.hour",
			"minute": "$_id.minute",
		}}, t}}}}

	case "hour":
		groupID = bson.M{
			"year":  bson.M{"$year": date},
			"day":   bson.M{"$dayOfMonth": date},
			"month": bson.M{"$month": date},
			"hour": bson.M{
				"$subtract": []interface{}{
					bson.M{"$hour": date},
					bson.M{"$mod": []interface{}{bson.M{"$hour": date}, duration}}}}}

		addFields = bson.M{"$addFields": bson.M{"ts": bson.M{"$subtract": []interface{}{bson.M{"$dateFromParts": bson.M{
			"year":  "$_id.year",
			"month": "$_id.month",
			"day":   "$_id.day",
			"hour":  "$_id.hour",
		}}, t}}}}

	case "day":
		groupID = bson.M{
			"year":  bson.M{"$year": date},
			"month": bson.M{"$month": date},
			"day": bson.M{
				"$subtract": []interface{}{
					bson.M{"$dayOfMonth": date},
					bson.M{"$mod": []interface{}{bson.M{"$dayOfMonth": date}, duration}}}}}

		addFields = bson.M{"$addFields": bson.M{"ts": bson.M{"$subtract": []interface{}{bson.M{"$dateFromParts": bson.M{
			"year":  "$_id.year",
			"month": "$_id.month",
			"day":   "$_id.day",
		}}, t}}}}

	case "week":
		groupID = bson.M{
			"year": bson.M{"$isoWeekYear": date},
			"isoWeek": bson.M{
				"$subtract": []interface{}{
					bson.M{"$isoWeek": date},
					bson.M{"$mod": []interface{}{bson.M{"$isoWeek": date}, duration}}}}}

		addFields = bson.M{"$addFields": bson.M{"ts": bson.M{"$subtract": []interface{}{bson.M{"$dateFromParts": bson.M{
			"isoWeekYear": "$_id.year",
			"isoWeek":     "$_id.isoWeek",
		}}, t}}}}

	case "month":
		groupID = bson.M{
			"year": bson.M{"$year": date},
			"month": bson.M{
				"$subtract": []interface{}{
					bson.M{
						"$multiply": []interface{}{
							bson.M{"$ceil": bson.M{"$divide": []interface{}{
								bson.M{"$month": date},
								duration}},
							},
							duration},
					}, duration - 1}}}

		addFields = bson.M{"$addFields": bson.M{"ts": bson.M{"$subtract": []interface{}{bson.M{"$dateFromParts": bson.M{
			"year":  "$_id.year",
			"month": "$_id.month",
		}}, t}}}}

	case "year":
		groupID = bson.M{
			"year": bson.M{
				"$subtract": []interface{}{
					bson.M{"$year": date},
					bson.M{"$mod": []interface{}{bson.M{"$year": date}, duration}},
				},
			},
		}

		addFields = bson.M{"$addFields": bson.M{"ts": bson.M{"$subtract": []interface{}{bson.M{"$dateFromParts": bson.M{
			"year": "$_id.year"}}, t}}}}

	}

	groupID["pair"] = "$pairName"
	groupID["baseToken"] = "$baseToken"
	groupID["quoteToken"] = "$quoteToken"
	group["_id"] = groupID

	return group, addFields
}
