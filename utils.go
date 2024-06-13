package main

import (
	"io"
	"net/http"
	"net/url"

	"github.com/rs/zerolog/log"
)

func fetch(symbol string) ([]byte, error) {
	u, err := url.Parse(source)
	if err != nil {
		log.Error().Msgf("error parsing source url: %v", err)
		return nil, err
	}
	params := url.Values{}
	params.Add(symbolParamKey, symbol)
	u.RawQuery = params.Encode()

	resp, err := http.Get(u.String()) // дефолтный клиент под капотом может зависнуть навсегда, так как у него нет таймаута
	if err != nil {
		log.Error().Msgf("error sending get request: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Msgf("error reading response body: %v", err) // если возвращаешь ошибку, логируй тогда ее там, куда возвращаешь
		return nil, err                                          // неплохо бы юзать errors.Wrap
	}

	return body, nil
}
