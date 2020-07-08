/*
 * === This file is part of ALICE O² ===
 *
 * Copyright 2018 CERN and copyright holders of ALICE O².
 * Author: Teo Mrnjavac <teo.mrnjavac@cern.ch>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 * In applying this license CERN does not waive the privileges and
 * immunities granted to it by virtue of its status as an
 * Intergovernmental Organization or submit itself to any jurisdiction.
 */

package channel

import (
	"strconv"
	"strings"

	"github.com/AliceO2Group/Control/core/controlcommands"
	"github.com/imdario/mergo"
	"github.com/sirupsen/logrus"
)

type Outbound struct {
	Channel
	Target string `json:"target" yaml:"target"`
}

func (outbound *Outbound) UnmarshalYAML(unmarshal func(interface{}) error) (err error) {
	target := struct {
		Target string `json:"target" yaml:"target"`
	}{}
	err = unmarshal(&target)
	if err != nil {
		return
	}

	ch := Channel{}
	err = unmarshal(&ch)
	if err != nil {
		return
	}

	outbound.Target = target.Target
	outbound.Channel = ch
	return
}

func (outbound Outbound) MarshalYAML() (interface{}, error) {
	// Flatten Outbound to have Target and Channel elements on the same "level"
	type _outbound struct {
		Name        string        `yaml:"name"`
		Type        ChannelType   `yaml:"type"`
		SndBufSize  int           `yaml:"sndBufSize,omitempty"`
		RcvBufSize  int           `yaml:"rcvBufSize,omitempty"`
		RateLogging string        `yaml:"rateLogging,omitempty"`
		Transport   TransportType `yaml:"transport"`
		Target      string        `yaml:"target"`
	}

	auxOutbound := _outbound{
		Name:       outbound.Channel.Name,
		Type:       outbound.Channel.Type,
		SndBufSize: outbound.Channel.SndBufSize,
		RcvBufSize: outbound.Channel.RcvBufSize,
		Transport:  outbound.Channel.Transport,
		Target:     outbound.Target,
	}

	if outbound.Channel.RateLogging == 0 {
		auxOutbound.RateLogging = ""
	} else {
		auxOutbound.RateLogging = strconv.Itoa(outbound.Channel.RateLogging)
	}

	return auxOutbound, nil
}

/*
FairMQ outbound channel property map example:
chans.data1.0.address       = tcp://localhost:5555                                                                                                                                                                                                                                                                                                                                                                                                 <string>      [provided]
chans.data1.0.method        = connect                                                                                                                                                                                                                                                                                                                                                                                                              <string>      [provided]
chans.data1.0.rateLogging   = 0                                                                                                                                                                                                                                                                                                                                                                                                                    <int>         [provided]
chans.data1.0.rcvBufSize    = 1000                                                                                                                                                                                                                                                                                                                                                                                                                 <int>         [provided]
chans.data1.0.rcvKernelSize = 0                                                                                                                                                                                                                                                                                                                                                                                                                    <int>         [provided]
chans.data1.0.sndBufSize    = 1000                                                                                                                                                                                                                                                                                                                                                                                                                 <int>         [provided]
chans.data1.0.sndKernelSize = 0                                                                                                                                                                                                                                                                                                                                                                                                                    <int>         [provided]
chans.data1.0.transport     = default                                                                                                                                                                                                                                                                                                                                                                                                              <string>      [provided]
chans.data1.0.type          = pull                                                                                                                                                                                                                                                                                                                                                                                                                 <string>      [provided]
chans.data1.numSockets      = 1
*/

func (outbound *Outbound) ToFMQMap(bindMap BindMap) (pm controlcommands.PropertyMap) {
	if outbound == nil {
		return
	}

	var address string
	var transport TransportType
	// If an explicit target was provided, we use it
	if strings.HasPrefix(outbound.Target, "tcp://") ||
		strings.HasPrefix(outbound.Target, "ipc://") {
		address = outbound.Target
		transport = outbound.Transport
	} else {
		// we don't need class.Bind data for this one, only task.bindPorts after resolving paths!
		for chPath, endpoint := range bindMap {
			// FIXME: implement more sophisticated channel matching here
			if outbound.Target == chPath {

				// We have a match, so we generate a resolved target address and break
				address = endpoint.GetAddress()
				transport = endpoint.GetTransport()
				break
			}
		}
	}

	if len(address) == 0 {
		return
	}

	return outbound.buildFMQMap(address, transport)
}

func (outbound *Outbound) buildFMQMap(address string, transport TransportType) (pm controlcommands.PropertyMap) {
	pm = make(controlcommands.PropertyMap)
	const chans = "chans"
	chName := outbound.Name
	// We assume one socket per channel, so this must always be set
	pm[strings.Join([]string{chans, chName, "numSockets"}, ".")] = "1"
	prefix := strings.Join([]string{chans, chName, "0"}, ".")

	chanProps := controlcommands.PropertyMap{
		"address":       address,
		"method":        "connect",
		"rateLogging":   strconv.Itoa(outbound.RateLogging),
		"rcvBufSize":    strconv.Itoa(outbound.RcvBufSize),
		"rcvKernelSize": "0", //NOTE: hardcoded
		"sndBufSize":    strconv.Itoa(outbound.SndBufSize),
		"sndKernelSize": "0", //NOTE: hardcoded
		"transport":     transport.String(),
		"type":          outbound.Type.String(),
	}

	if (transport != outbound.Transport) &&
		(outbound.Transport != DEFAULT) {
		log.WithFields(logrus.Fields{
			"address":                address,
			"oubound":                outbound.Name,
			"actualInboundTransport": transport,
			"outboundTransport":      outbound.Transport,
		}).
			Warn("channel transport mismatch, fix workflow template")
	}

	for k, v := range chanProps {
		pm[prefix+"."+k] = v
	}
	return
}

func MergeOutbound(hp, lp []Outbound) (channels []Outbound) {
	channels = make([]Outbound, len(hp))
	copy(channels, hp)

	for _, v := range lp {
		updated := false
		for _, pCh := range channels {
			if v.Name == pCh.Name {
				mergo.Merge(&pCh, v)
				updated = true
				break
			}
		}
		if !updated {
			channels = append(channels, v)
		}
	}

	return
}
