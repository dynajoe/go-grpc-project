package util

import (
	"github.com/go-playground/validator"
	"github.com/kelseyhightower/envconfig"
)

func Load(spec interface{}) error {
	if err := envconfig.Process("", spec); err != nil {
		return err
	}
	if err := validator.New().Struct(spec); err != nil {
		return err
	}

	return nil
}
