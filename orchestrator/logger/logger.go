package logger

import "go.uber.org/zap"

var Logger *zap.SugaredLogger

func init() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic("Unable to initialize logger!")
	}
	Logger = logger.Sugar()
}
