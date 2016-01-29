// Core service infrastructure for servicing starting/stopping/SIGTERM, and heartbeating etc
package service

import (
	proto "github.com/cyanly/gotrade/proto/service"
	"github.com/nats-io/nats"

	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Service struct {
	Config Config
	Status proto.Heartbeat_Status

	shutdownChannel chan bool
}

func NewService(c Config) *Service {
	uuid = fmt.Sprint(hostname, ":", pid)
	log.Println("Service [", c.ServiceName, "] starting @ ", uuid)

	svc := &Service{
		Config:          c,
		Status:          proto.STARTING,
		shutdownChannel: make(chan bool),
	}

	messageBus, err := nats.Connect(svc.Config.MessageBusURL)
	if err != nil {
		log.Fatal("error: Cannot connect to message bus @ ", svc.Config.MessageBusURL)
	}

	//Heartbeating
	currDateTime := time.Now().UTC().Format(time.RFC3339)
	hbMsg := &proto.Heartbeat{
		Name:             &svc.Config.ServiceName,
		Id:               &uuid,
		Status:           proto.STARTING,
		Machine:          &hostname,
		CreationDatetime: &currDateTime,
		CurrentDatetime:  &currDateTime,
	}
	hbTicker := time.NewTicker(time.Second * time.Duration(svc.Config.HeartbeatFreq))
	go func(shutdownChannel chan bool) {
		publish_address := "service.Heartbeat." + svc.Config.ServiceName

		for range hbTicker.C {
			currDateTime := time.Now().UTC().Format(time.RFC3339)
			hbMsg.CurrentDatetime = &currDateTime
			hbMsg.Status = svc.Status

			if data, _ := hbMsg.Marshal(); data != nil {
				messageBus.Publish(publish_address, data)
			}

			select {
			case <-shutdownChannel:
				hbTicker.Stop()

				//Publish Stop heartbeat
				if svc.Status != proto.ERROR {
					svc.Status = proto.STOPPED
				}
				currDateTime := time.Now().UTC().Format(time.RFC3339)
				hbMsg.CurrentDatetime = &currDateTime
				hbMsg.Status = svc.Status
				if data, _ := hbMsg.Marshal(); data != nil {
					messageBus.Publish(publish_address, data)
				}

				messageBus.Close()

				log.Println("Server Terminated")
				return
			}
		}
	}(svc.shutdownChannel)

	return svc
}

func (self *Service) Start() chan bool {
	//SIGINT or SIGTERM is caught
	quitChannel := make(chan os.Signal)
	signal.Notify(quitChannel, syscall.SIGINT, syscall.SIGTERM)
	shutdownCallerChannel := make(chan bool)
	go func() {
		<-quitChannel
		self.shutdownChannel <- true
		shutdownCallerChannel <- true
	}()

	self.Status = proto.RUNNING

	log.Println("Service [", self.Config.ServiceName, "] Started")
	return shutdownCallerChannel
}