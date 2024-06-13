package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// В идеале брать все константные значения из конфига/энвов
const (
	configFile     = "config.yaml"
	source         = "https://api.binance.com/api/v3/ticker/price"
	symbolParamKey = "symbol"
	maxTryCount    = 10
	stopWord       = "STOP"
)

var (
	totalReqCount int
	totalMx       sync.RWMutex

	// В иделе кэш должен быть реализоване в Redis
	cache   map[string]string = make(map[string]string)
	cacheMx sync.Mutex        // в го принято ставить мьютекс выше тех полей, которые он защищает
)

type PriceResponse struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

// https://stackoverflow.com/questions/38842457/interface-naming-convention-golang
type IHandler interface {
	Run(ctx context.Context, symbols []string)
	GetRequestsCount() int
}

type Handler struct {
	maxTries   int
	requests   int
	requestsMx sync.RWMutex
}

func (h *Handler) Run(ctx context.Context, symbols []string) {
	var retryCount int // YAGNY
	for _, s := range symbols {
		select {
		case <-ctx.Done():
			log.Info().Msg("worker stopping by request")
			return
		default:
		}

		if retryCount > h.maxTries {
			log.Error().Msgf("exceeded max retry count")
			return
		}

		h.addToCount() // не понял зачем тебе сразу и addToCount() и totalReqCount++ понадобилось - дублируешь одну и ту же логику

		body, err := fetch(s) // если метод большой, то лучше назвать переменную s понятнее
		if err != nil {
			retryCount++
			continue
		}
		retryCount = 0

		var priceResp PriceResponse
		if err := json.Unmarshal(body, &priceResp); err != nil { // ты размазал логику получения цены - fetch и тут
			log.Error().Msgf("error unmarshaling response: %v", err)
			continue
		}

		totalMx.Lock()
		totalReqCount++ // не учитываешь неудачные запросы
		totalMx.Unlock()

		msgFormat := "%s price:%s"
		cacheMx.Lock() // почему бы не воспользоваться одним мьютексом, вместо двух
		_, ok := cache[priceResp.Symbol]
		if ok { // всегда будешь писать changed - даже когда цена не менялась
			msgFormat += " changed"
		}
		cache[priceResp.Symbol] = priceResp.Price
		cacheMx.Unlock()

		log.Info().Msgf(msgFormat, priceResp.Symbol, priceResp.Price) // в методе run нельзя писать цены - в задаче указано
	}
}

func (h *Handler) GetRequestsCount() int {
	h.requestsMx.RLock()
	defer h.requestsMx.RUnlock()
	return h.requests
}

func (h *Handler) addToCount() {
	h.requestsMx.Lock()
	defer h.requestsMx.Unlock()
	h.requests++
}

func main() {
	var cfg config
	if err := cfg.getConfig(); err != nil {
		log.Fatal().Msg("error getting config")
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}

	split, extra := len(cfg.Symbols)/cfg.MaxWorkers, len(cfg.Symbols)%cfg.MaxWorkers
	start := 0
	for i := 0; i < cfg.MaxWorkers; i++ {
		end := start + split
		if i < extra {
			end++
		}

		// Объявляем отдельный хэндлер для каждого воркера чтобы трекать кол-во реквестов по отдельности
		h := &Handler{
			maxTries: maxTryCount,
		}

		wg.Add(1)
		go func(s, e, num int, wg *sync.WaitGroup) { // очень неудачные названия параметров
			defer wg.Done()
			h.Run(ctx, cfg.Symbols[s:e]) // я бы разнес группировку символов и запуск воркеров - это было бы легче тестировать

			log.Info().Msgf("handler %d made %d requests", num, h.GetRequestsCount()) // YAGNY - задание звучало не так
		}(start, end, i, &wg)

		start = end
	}

	// Сканнер ждущий стоп слово
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			log.Info().Msg(scanner.Text())
			if scanner.Text() == stopWord {
				cancel()
				break
			}
		}
		if scanner.Err() != nil {
			log.Error().Msgf("error reading from stdin: %v", scanner.Err())
		}
	}()

	// Тикер на вывод общего количества реквестов. Работает каждые 5 секунд
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			totalMx.RLock()
			log.Info().Msgf("workers requests total: %d", totalReqCount)
			totalMx.RUnlock()
		}
	}()

	wg.Wait()
	ticker.Stop()

	log.Info().Msg("all work is done")
}
