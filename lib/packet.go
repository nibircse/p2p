package ptp

/*
	Know packet types:
		512   (PUP)
		2048  (IP)
		2054  (ARP)
		32821 (RARP)
		33024 (802.1q)
		34525 (IPv6)
		34915 (PPPOE discovery)
		34916 (PPPOE session)
*/

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/mdlayher/ethernet"
)

// PacketType is a type of the IPv4 packet
type PacketType int

// PacketHandlerCallback represents a callback function for each packet type
type PacketHandlerCallback func(data []byte, proto int) error

// Packet Types
const (
	EthPacketSize        int        = 512
	PacketPARCUniversal  PacketType = 512
	PacketIPv4           PacketType = 2048
	PacketARP            PacketType = 2054
	PacketRARP           PacketType = 32821
	Packet8021Q          PacketType = 33024
	PacketIPv6           PacketType = 34525
	PacketPPPoEDiscovery PacketType = 34915
	PacketPPPoESession   PacketType = 34916
	PacketLLDP           PacketType = 35020
)

var (
	// ErrInvalidHardwareAddr is returned when one or more invalid hardware
	// addresses are passed to NewPacket.
	ErrInvalidHardwareAddr = errors.New("invalid hardware address")

	// ErrInvalidIP is returned when one or more invalid IPv4 addresses are
	// passed to NewPacket.
	ErrInvalidIP = errors.New("invalid IPv4 address")

	// errInvalidARPPacket is returned when an ethernet frame does not
	// indicate that an ARP packet is contained in its payload.
	errInvalidARPPacket = errors.New("invalid ARP packet")

	//PacketHandlers map[PacketType]PacketHandlerCallback

	// PacketID is a ID of the received packet
	PacketID uint16
)

// Operation determines whether operation is a request or a reply
type Operation uint16

// Types of Operation
const (
	OperationRequest Operation = 1
	OperationReply   Operation = 2
)

// ARPPacket represents an ARP packet
type ARPPacket struct {
	// HardwareType specifies an IANA-assigned hardware type, as described
	// in RFC 826.
	HardwareType uint16

	// ProtocolType specifies the internetwork protocol for which the ARP
	// request is intended.  Typically, this is the IPv4 EtherType.
	ProtocolType uint16

	// HardwareAddrLength specifies the length of the sender and target
	// hardware addresses included in a Packet.
	HardwareAddrLength uint8

	// IPLength specifies the length of the sender and target IPv4 addresses
	// included in a Packet.
	IPLength uint8

	// Operation specifies the ARP operation being performed, such as request
	// or reply.
	Operation Operation

	// SenderHardwareAddr specifies the hardware address of the sender of this
	// Packet.
	SenderHardwareAddr net.HardwareAddr

	// SenderIP specifies the IPv4 address of the sender of this Packet.
	SenderIP net.IP

	// TargetHardwareAddr specifies the hardware address of the target of this
	// Packet.
	TargetHardwareAddr net.HardwareAddr

	// TargetIP specifies the IPv4 address of the target of this Packet.
	TargetIP net.IP
}

// Handles a packet that was received by TUN/TAP device
// Receiving a packet by device means that some application sent a network
// packet within a subnet in which our application works.
// This method calls appropriate gorouting for extracted packet protocol
func (p *PeerToPeer) handlePacket(contents []byte, proto int) error {
	callback, exists := p.PacketHandlers[PacketType(proto)]
	if exists {
		return callback(contents, proto)
	}
	Log(Warning, "Captured undefined packet: %d", PacketType(proto))
	return fmt.Errorf("Captured undefined packet: %d", PacketType(proto))
}

// Handles a IPv4 packet and sends it to it's destination
func (p *PeerToPeer) handlePacketIPv4(contents []byte, proto int) error {
	f := new(ethernet.Frame)
	if err := f.UnmarshalBinary(contents); err != nil {
		Log(Error, "Failed to unmarshal IPv4 packet")
		return fmt.Errorf("Failed to unmarshal IPv4 packet")
	}
	if f.EtherType != ethernet.EtherTypeIPv4 {
		return fmt.Errorf("Wrong packet type in IPv4 handler. Got %d. Expecting %d", f.EtherType, ethernet.EtherTypeIPv4)
	}

	msg, err := p.CreateMessage(MsgTypeNenc, contents, uint16(proto), true)
	if err == nil && msg != nil {
		_, err = p.SendTo(f.Destination, msg)
		return err
	}
	return err
}

// TODO: Implement IPv6 Support
func (p *PeerToPeer) handlePacketIPv6(contents []byte, proto int) error {
	return nil
}

// TODO: Implement PARC Universal Support
func (p *PeerToPeer) handlePARCUniversalPacket(contents []byte, proto int) error {
	return nil
}

// TODO: Implement RARP Support
func (p *PeerToPeer) handleRARPPacket(contents []byte, proto int) error {
	return nil
}

// TODO: Implement 802.1q Support
func (p *PeerToPeer) handle8021qPacket(contents []byte, proto int) error {
	return nil
}

// TODO: Implement PPPoE Discovery Support
func (p *PeerToPeer) handlePPPoEDiscoveryPacket(contents []byte, proto int) error {
	return nil
}

// TODO: Implement PPPoE Session Support
func (p *PeerToPeer) handlePPPoESessionPacket(contents []byte, proto int) error {
	return nil
}

func (p *PeerToPeer) handlePacketARP(contents []byte, proto int) error {
	// Prepare new ethernet frame and fill it with
	// contents of the packet
	f := new(ethernet.Frame)
	if err := f.UnmarshalBinary(contents); err != nil {
		Log(Error, "Failed to Unmarshal ARP Binary")
		return fmt.Errorf("failed to unmarshal ARP binary: %s", err.Error())
	}

	packet := new(ARPPacket)
	if err := packet.UnmarshalARP(f.Payload); err != nil {
		Log(Error, "Failed to unmarshal arp")
		return fmt.Errorf("failed to unmarshal ARP packet: %s", err.Error())
	}
	if p.Swarm == nil {
		return fmt.Errorf("nil peer list")
	}

	id, err := p.Swarm.GetID(packet.TargetIP.String())
	if err != nil {
		Log(Trace, "Unknown IP requested: %s", packet.TargetIP.String())
		return fmt.Errorf("requested unknown IP: %s", packet.TargetIP)
	}
	peer := p.Swarm.GetPeer(id)
	if peer == nil {
		Log(Debug, "Can't lookup address: Specified peer was not found")
		return fmt.Errorf("peer not found during arp request: %s", id)
	}
	hwAddr := peer.PeerHW
	if hwAddr == nil {
		Log(Error, "Cannot find hardware address for requested IP")
		_, hwAddr = GenerateMAC()
		peer.PeerHW = hwAddr
		p.Swarm.Update(id, peer)
	}
	if hwAddr.String() == "00:00:00:00:00:00" {
		_, hwAddr = GenerateMAC()
		peer.PeerHW = hwAddr
		p.Swarm.Update(id, peer)
	}
	var reply ARPPacket
	ip := net.ParseIP(packet.TargetIP.String())
	response, err := reply.NewPacket(OperationReply, hwAddr, ip, packet.SenderHardwareAddr, packet.SenderIP)
	if err != nil {
		Log(Error, "Failed to create ARP response")
		return fmt.Errorf("failed to create app response: %s", err.Error())
	}
	rp, err := response.MarshalBinary()
	if err != nil {
		Log(Error, "Failed to marshal ARP response packet")
		return fmt.Errorf("failed to marshal arp response binary: %s", err.Error())
	}

	fr := &ethernet.Frame{
		Destination: response.TargetHardwareAddr,
		Source:      response.SenderHardwareAddr,
		EtherType:   ethernet.EtherTypeARP,
		Payload:     rp,
	}

	fb, err := fr.MarshalBinary()
	if err != nil {
		Log(Error, "Failed to marshal ARP Ethernet Frame")
		return fmt.Errorf("failed to marshal ARP ethernet frame: %s", err.Error())
	}
	Log(Trace, "%v", packet.String())
	return p.WriteToDevice(fb, uint16(proto), false)
}

func (p *PeerToPeer) handlePacketLLDP(contents []byte, proto int) error {
	return nil
}

func (p *ARPPacket) String() string {
	return fmt.Sprintf("HWType %d, Proto: %d, HWAddrLength: %d, IPLength: %d, Operation: %d, SHWAddr: %s, SIP: %s, THWAddr: %s, TIP: %s", p.HardwareType, p.ProtocolType, p.HardwareAddrLength, p.IPLength, p.Operation, p.SenderHardwareAddr.String(), p.SenderIP.String(), p.TargetHardwareAddr.String(), p.TargetIP.String())
}

// MarshalBinary allocates a byte slice containing the data from a Packet.
func (p *ARPPacket) MarshalBinary() ([]byte, error) {
	// 2 bytes: hardware type
	// 2 bytes: protocol type
	// 1 byte : hardware address length
	// 1 byte : protocol length
	// 2 bytes: operation
	// N bytes: source hardware address
	// N bytes: source protocol address
	// N bytes: target hardware address
	// N bytes: target protocol address

	// Though an IPv4 address should always 4 bytes, go-fuzz
	// very quickly created several crasher scenarios which
	// indicated that these values can lie.
	b := make([]byte, 2+2+1+1+2+(p.IPLength*2)+(p.HardwareAddrLength*2))

	// Marshal fixed length data

	binary.BigEndian.PutUint16(b[0:2], p.HardwareType)
	binary.BigEndian.PutUint16(b[2:4], p.ProtocolType)

	b[4] = p.HardwareAddrLength
	b[5] = p.IPLength

	binary.BigEndian.PutUint16(b[6:8], uint16(p.Operation))

	// Marshal variable length data at correct offset using lengths
	// defined in p

	n := 8
	hal := int(p.HardwareAddrLength)
	pl := int(p.IPLength)

	copy(b[n:n+hal], p.SenderHardwareAddr)
	n += hal

	copy(b[n:n+pl], p.SenderIP)
	n += pl

	copy(b[n:n+hal], p.TargetHardwareAddr)
	n += hal

	copy(b[n:n+pl], p.TargetIP)

	return b, nil
}

// UnmarshalARP unmarshals ARP header
func (p *ARPPacket) UnmarshalARP(b []byte) error {
	// Must have enough room to retrieve both hardware address and IP addresses
	if len(b) < 28 {
		return io.ErrUnexpectedEOF
	}

	// Sender hardware address
	p.SenderHardwareAddr = b[8:14]

	// Sender IP address
	p.SenderIP = b[14:18]

	// Target hardware address
	p.TargetHardwareAddr = b[18:24]

	// Target IP address
	p.TargetIP = b[24:28]

	return nil
}

// NewPacket creates new ARP packet
func (p *ARPPacket) NewPacket(op Operation, srcHW net.HardwareAddr, srcIP net.IP, dstHW net.HardwareAddr, dstIP net.IP) (*ARPPacket, error) {
	// Validate hardware addresses for minimum length, and matching length
	if len(srcHW) < 6 {
		return nil, ErrInvalidHardwareAddr
	}
	if len(dstHW) < 6 {
		return nil, ErrInvalidHardwareAddr
	}
	if len(srcHW) != len(dstHW) {
		return nil, ErrInvalidHardwareAddr
	}

	// Validate IP addresses to ensure they are IPv4 addresses, and
	// correct length
	srcIP = srcIP.To4()
	if srcIP == nil {
		return nil, ErrInvalidIP
	}
	dstIP = dstIP.To4()
	if dstIP == nil {
		return nil, ErrInvalidIP
	}

	return &ARPPacket{
		// There is no Go-native way to detect hardware type of a network
		// interface, so default to 1 (ethernet 10Mb) for now
		HardwareType: 1,

		// Default to EtherType for IPv4
		ProtocolType: uint16(ethernet.EtherTypeIPv4),

		// Populate other fields using input data
		HardwareAddrLength: uint8(len(srcHW)),
		IPLength:           uint8(len(srcIP)),
		Operation:          op,
		SenderHardwareAddr: srcHW,
		SenderIP:           srcIP,
		TargetHardwareAddr: dstHW,
		TargetIP:           dstIP,
	}, nil
}
