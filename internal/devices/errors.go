package devices

import (
	"errors"
	"strings"
)

func ejectError(out []byte) error {
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = "não foi possível ejetar; feche arquivos abertos no dispositivo e tente de novo"
	}
	return errors.New(msg)
}
