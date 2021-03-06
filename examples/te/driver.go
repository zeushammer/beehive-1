package main

import (
	"fmt"
	"math/rand"

	"github.com/kandoo/beehive"
	"github.com/kandoo/beehive/Godeps/_workspace/src/github.com/golang/glog"
)

type FlowStat struct {
	Flow  Flow
	Bytes uint64
}

type SwitchState struct {
	Switch Switch
	Flows  []FlowStat
}

type FlowMod struct {
	Switch Switch
	Flow   Flow
}

const (
	switchStateDict = "SwitchState"
)

type Driver struct {
	switches map[Switch]SwitchState
}

func NewDriver(startingSwitchId, numberOfSwitches int) *Driver {
	d := &Driver{make(map[Switch]SwitchState)}
	for i := 0; i < numberOfSwitches; i++ {
		sw := Switch(startingSwitchId + i)
		state := SwitchState{Switch: sw}
		state.Flows = append(state.Flows, FlowStat{Flow{1, 1, 2}, 100})
		d.switches[sw] = state
	}
	return d
}

func (d *Driver) Start(ctx beehive.RcvContext) {
	for s, _ := range d.switches {
		ctx.Emit(SwitchJoined{s})
	}
}

func (d *Driver) Stop(ctx beehive.RcvContext) {
}

func (d *Driver) Rcv(m beehive.Msg, ctx beehive.RcvContext) error {
	if m.NoReply() {
		return nil
	}

	q, ok := m.Data().(StatQuery)
	if !ok {
		return nil
	}

	s, ok := d.switches[q.Switch]
	if !ok {
		return fmt.Errorf("No switch stored in the driver: %+v", s)
	}

	for i, f := range s.Flows {
		f.Bytes += uint64(rand.Intn(maxSpike))
		s.Flows[i] = f
		glog.V(2).Infof("Emitting stat result for %+v", f)
		ctx.Emit(StatResult{q, f.Flow, f.Bytes})
	}

	d.switches[q.Switch] = s
	return nil
}

func (d *Driver) Map(m beehive.Msg,
	ctx beehive.MapContext) beehive.MappedCells {

	var k string
	switch d := m.Data().(type) {
	case StatQuery:
		k = d.Switch.Key()
	case FlowMod:
		k = d.Switch.Key()
	}
	return beehive.MappedCells{{switchStateDict, k}}
}
