package device

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"go.bug.st/serial"
)

// TransportConfig selects how the controller is reached: the (W)LAN modem on
// TCP port 4001, or the RS232 "Modem COM" port (-X26) with a null-modem cable.
type TransportConfig struct {
	Type     string `yaml:"type" json:"type"` // "tcp" or "serial"
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	Serial   string `yaml:"serialPort" json:"serialPort"`
	Baud     int    `yaml:"baud" json:"baud"`
	DataBits int    `yaml:"dataBits" json:"dataBits"`
	StopBits int    `yaml:"stopBits" json:"stopBits"` // 1 or 2
}

// Defaults taken from homeControl's settings (LAN_Port 4001, 19200 8N1).
func (c *TransportConfig) ApplyDefaults() {
	if c.Type == "" {
		c.Type = "tcp"
	}
	if c.Port == 0 {
		c.Port = 4001
	}
	if c.Baud == 0 {
		c.Baud = 19200
	}
	if c.DataBits == 0 {
		c.DataBits = 8
	}
	if c.StopBits == 0 {
		c.StopBits = 1
	}
}

func (c TransportConfig) String() string {
	if c.Type == "serial" {
		return fmt.Sprintf("serial %s %d baud", c.Serial, c.Baud)
	}
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

func openTransport(ctx context.Context, c TransportConfig) (io.ReadWriteCloser, error) {
	switch c.Type {
	case "tcp", "":
		if c.Host == "" {
			return nil, fmt.Errorf("no host configured")
		}
		d := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
		return d.DialContext(ctx, "tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
	case "serial":
		if c.Serial == "" {
			return nil, fmt.Errorf("no serial port configured")
		}
		stop := serial.OneStopBit
		if c.StopBits == 2 {
			stop = serial.TwoStopBits
		}
		return serial.Open(c.Serial, &serial.Mode{BaudRate: c.Baud, DataBits: c.DataBits, Parity: serial.NoParity, StopBits: stop})
	default:
		return nil, fmt.Errorf("unknown transport %q", c.Type)
	}
}

// SerialPorts lists the serial ports present on this machine.
func SerialPorts() []string {
	ports, _ := serial.GetPortsList()
	return ports
}
