package tool

import "agent/teams"

var MessageBus *teams.MessageBus

func SetMessageBus(bus *teams.MessageBus) {
	MessageBus = bus
}
