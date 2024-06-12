package main

import (
	"os"
	"runtime"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type config struct {
	Symbols    []string `yaml:"symbols"`
	MaxWorkers int      `yaml:"max_workers"`
}

func (c *config) getConfig() error {
	yamlConfig, err := os.ReadFile(configFile)
	if err != nil {
		log.Error().Msgf("error reading config file: %v", err)
		return err
	}

	if err := yaml.Unmarshal(yamlConfig, c); err != nil {
		log.Error().Msgf("error unmarshaling config: %v", err)
		return err
	}

	if c.MaxWorkers < 1 {
		log.Error().Msg("max workers' minimun value is 1")
		return err
	}

	// Может я не так понял задание, но я тут не вижу смысла ограничивать количество горутин
	// максимальным количество ядер на процессоре, т.к. при превышении этого количества горутины
	// будут работать асинхронно + параллельно, что в итоге будет быстрее все равно
	if c.MaxWorkers > runtime.NumCPU() {
		c.MaxWorkers = runtime.NumCPU()
	}

	return nil
}
