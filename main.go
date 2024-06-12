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
	cacheMx sync.Mutex
)

type PriceResponse struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

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
	var retryCount int
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

		h.addToCount()

		body, err := fetch(s)
		if err != nil {
			retryCount++
			continue
		}
		retryCount = 0

		var priceResp PriceResponse
		if err := json.Unmarshal(body, &priceResp); err != nil {
			log.Error().Msgf("error unmarshaling response: %v", err)
			continue
		}

		totalMx.Lock()
		totalReqCount++
		totalMx.Unlock()

		msgFormat := "%s price:%s"
		cacheMx.Lock()
		_, ok := cache[priceResp.Symbol]
		if ok {
			msgFormat += " changed"
		}
		cache[priceResp.Symbol] = priceResp.Price
		cacheMx.Unlock()

		log.Info().Msgf(msgFormat, priceResp.Symbol, priceResp.Price)
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
		go func(s, e, num int, wg *sync.WaitGroup) {
			defer wg.Done()
			h.Run(ctx, cfg.Symbols[s:e])

			log.Info().Msgf("handler %d made %d requests", num, h.GetRequestsCount())
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
