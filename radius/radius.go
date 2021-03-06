package radius

import (
	rad_api "github.com/meklis/all-ok-radius-server/api"
	"github.com/meklis/all-ok-radius-server/logger"
	"layeh.com/radius"
	"log"
	"os"
)

type Radius struct {
	lg           *logger.Logger
	listenAddr   string
	secret       string
	agentParsing bool
	api          *rad_api.Api
}

func Init() *Radius {
	rad := new(Radius)
	rad.listenAddr = "0.0.0.0:1812"
	rad.secret = "secret"
	rad.agentParsing = true
	rad.lg, _ = logger.New("radius", 0, os.Stdout)
	return rad
}
func (rad *Radius) SetLogger(lg *logger.Logger) *Radius {
	rad.lg = lg
	return rad
}
func (rad *Radius) SetListenAddr(listenAddr string) *Radius {
	rad.listenAddr = listenAddr
	return rad
}
func (rad *Radius) SetSecret(secret string) *Radius {
	rad.secret = secret
	return rad
}
func (rad *Radius) SetAgentParsing(enabled bool) *Radius {
	rad.agentParsing = enabled
	return rad
}

func (rad *Radius) SetAPI(apiR *rad_api.Api) *Radius {
	rad.api = apiR
	return rad
}

func (rad *Radius) ListenAndServe() error {
	server := radius.PacketServer{
		Addr:         rad.listenAddr,
		Network:      "udp",
		SecretSource: radius.StaticSecretSource([]byte(rad.secret)),
		Handler:      radius.HandlerFunc(rad.handler),
	}

	rad.lg.InfoF("Starting radius server on %v", rad.listenAddr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
		return err
	}
	return nil
}
