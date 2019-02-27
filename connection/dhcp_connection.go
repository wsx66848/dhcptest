package connection

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"
	"math/rand"
	"dhcptest/utility"
	"dhcptest/layers"
	"github.com/google/gopacket"
)

const responseTimeout = time.Second * 5

var OnlyDiscover = false

type Callback func(*utility.Lease)

//DhcpClient
type DhcpClient struct {
	BindIP net.IP
	ClientMac net.HardwareAddr
	Hostname string
	Iface *net.Interface
	Lease *utility.Lease
	OnBound Callback
	OnExpire Callback
	DHCPOptions []layers.DHCPOption
	Timeout time.Duration
	laddr net.UDPAddr
	conn *TransPort
	logger *utility.Log
	ListenThreadPoolSize int
	listenThreadPool []UDPListenThread
	rebind bool
	shutdown bool
	running bool
	notify chan struct{}
	sign chan int
	wg sync.WaitGroup
}

var DefaultParamsRequestList = []layers.DHCPOpt{
	layers.DHCPOptSubnetMask,
	layers.DHCPOptRouter,
	layers.DHCPOptTimeServer,
	layers.DHCPOptDNS,
	layers.DHCPOptDomainName,
	layers.DHCPOptInterfaceMTU,
	layers.DHCPOptNTPServers,
}

func (dc *DhcpClient) AddOption(optType layers.DHCPOpt, data []byte) {
	dc.DHCPOptions  = append(dc.DHCPOptions, layers.NewDHCPOption(optType, data))
}

func (dc *DhcpClient) AddParamRequest(dhcpOpt layers.DHCPOpt) {
	for i := range dc.DHCPOptions {
		if dc.DHCPOptions[i].Type == layers.DHCPOptParamsRequest {
			dc.DHCPOptions[i].AddByte(byte(dhcpOpt))
			return
		}
	}
	dc.AddOption(layers.DHCPOptParamsRequest, []byte{byte(dhcpOpt)})
}

func (dc *DhcpClient) Start() {
	/*
	for _, param := range DefaultParamsRequestList {
		dc.AddParamRequest(param)
	}
	*/
	/*
	dc.AddOption(layers.DHCPOptHostname, []byte(dc.Hostname))

	if dc.notify != nil {
		log.Panicf("client for %s already started", dc.Iface.Name)
	}

	dc.notify = make(chan struct{})
	dc.laddr = net.UDPAddr{IP: dc.BindIP, Port: 68}
	dc.logger = &utility.Log{Logger: utility.DHCPLogger()}
	dc.wg.Add(1)
	fmt.Printf("client: %+v\n", dc)
	go dc.run()
	*/
	dc.init()
	//dc.sign = make(chan int)
	dc.laddr = net.UDPAddr{IP: dc.BindIP, Port: 68}
	dc.AddOption(layers.DHCPOptHostname, []byte(dc.Hostname))

}

func (dc *DhcpClient) init() {
	dc.logger = &utility.Log{Logger: utility.DHCPLogger()}
	/*
	dc.conn = &TransPort{
		Dialer: UDPDialer(),
		Listener: UDPListener(),
	}
	*/
	for i := 0; i < dc.ListenThreadPoolSize; i++ {
		lt := &ListenThread{
			Id: rand.Uint32(),
			Timeout: dc.Timeout,
			Listener: UDPListener(),
		}
		lt.init()
		dc.listenThreadPool = append(dc.listenThreadPool, lt)
	}
}

func (dc *DhcpClient) Stop() {
	//goroutine开启需要消耗时间，所以在shutdown之前调用同样耗时的Print,能保证run调用时shutdown还为false,这样runonce能够顺利执行
	//log.Printf("[%s] shutting down dhcp client", dc.Iface.Name)
	//未开始之前先不调用stop
	/*
	for !dc.running {}

	close(dc.notify)
	dc.shutdown = true

	//wait all goroutines stop
	dc.wg.Wait()

	log.Printf("[%s] shutting down dhcp client", dc.Iface.Name)
	dc.sign <- 1
	dc.wg.Wait()
	*/
	log.Printf("[%s] shutting down dhcp client", dc.Iface.Name)

}
/*

func (dc *DhcpClient) Renew() {
	select {
	case dc.notify <- struct{}{}:
	default:
	}
}

func (dc *DhcpClient) Rebind() {
	dc.rebind = true
	dc.Lease = nil
	dc.Renew()
}


func (dc *DhcpClient) run() {
	for !dc.shutdown {
		dc.running = true
	    dc.runOnce()
	}
	dc.wg.Done()
}

func (dc *DhcpClient) runOnce() {
	var err error
	if dc.Lease == nil || dc.rebind {
		if OnlyDiscover {
			err = dc.withConnection(dc.onlyDiscover)
		} else {
			err = dc.withConnection(dc.discoverAndRequest)
		}
		if err == nil {
			dc.rebind = false
		}
	} else {
		err = dc.withConnection(dc.renew)
	}

	if err != nil {
		log.Printf("[%s] error: %s", dc.Iface.Name, err)
		select {
		case <-dc.notify:
		case <-time.After(time.Second):
		}
		return
	}
	select {
	case <- dc.notify:
		return
	case <-time.After(time.Until(dc.Lease.Expire)):
			dc.unbound()
			break
    case <-time.After(time.Until(dc.Lease.Rebind)):
     		dc.rebind = true
     		break
    case <-time.After(time.Until(dc.Lease.Renew)):
    	break
	}
}

func (dc *DhcpClient) unbound() {
	if cb := dc.OnExpire; cb != nil {
		cb(dc.Lease)
	}
	dc.Lease = nil
}

func (dc *DhcpClient) withConnection(f func() error) error {
	conn := &TransPort{
		Dialer: UDPDialer(),
		Listener: UDPListener(),
	}

	dc.conn = conn
	//dc.xids = append(dc.xids, rand.Uint32())

	defer func() {
		dc.conn = nil
	}()


	return f()

}

func (dc *DhcpClient) onlyDiscover() error {
	_, err := dc.discover()
	if err!= nil {
		return err
	}
	return nil
}

func (dc *DhcpClient) discoverAndRequest() error {
	lease, err := dc.discover()
	if err != nil {
		return err
	}
	return dc.request(lease)
}

func (dc *DhcpClient) renew() error {
	return dc.request(dc.Lease)
}

func (dc *DhcpClient) request(lease *utility.Lease) error {
	fixedAddress :=  []byte(lease.FixedAddress)
	serverID := []byte(lease.ServerID)
	err := dc.sendPacket(layers.DHCPMsgTypeRequest, append(dc.DHCPOptions,
		layers.NewDHCPOption(layers.DHCPOptRequestIP, fixedAddress),
		layers.NewDHCPOption(layers.DHCPOptServerID, serverID),
	    ))
	if err != nil {
		return err
	}

	msgType, lease , err := dc.waitForResponse(layers.DHCPMsgTypeAck, layers.DHCPMsgTypeNak)

	if err != nil {
		return err
	}

	switch msgType {
	case layers.DHCPMsgTypeAck:
		if lease.Expire.IsZero() {
			err = errors.New("expire value is zero")
			break
		}

		if lease.Renew.IsZero() {
			lease.Renew = lease.Bound.Add(lease.Expire.Sub(lease.Bound) / 2)
		}

		if lease.Rebind.IsZero() {
			lease.Rebind = lease.Bound.Add(lease.Expire.Sub(lease.Bound) / 1000 * 875)
		}
		dc.Lease = lease

		if cb := dc.OnBound; cb != nil {
			cb(lease)
		}
		break
	case layers.DHCPMsgTypeNak:
		err = errors.New("received NAK")
		break
	default:
		err = fmt.Errorf("unexpected response: %s", msgType.String())
		break
	}
	return err
}
*/

func (dc *DhcpClient) getIdleListenThread() UDPListenThread {
	for _, lt := range dc.listenThreadPool {
		if lt.Status() == Sleeping {
			return lt
		}
	}
	return nil
}

func (dc *DhcpClient) SendDiscover(request bool) error {
	// start listen thread
	lt := dc.getIdleListenThread()
	if lt == nil {
		return fmt.Errorf("no idle listen thread now\n")
	}
	xid := rand.Uint32()
	c := make(chan interface{}, 10)
	lt.SetXid(xid)
	err := lt.Start(&dc.laddr, c, layers.DHCPMsgTypeOffer)
	if err != nil {
		return err
	}

	//generate packet
	var clientMac net.HardwareAddr
	if dc.ClientMac == nil {
		var err error
		clientMac, err = net.ParseMAC(utility.RandomMac())
		if err != nil {
			return err
		}
	} else {
		clientMac = dc.ClientMac
	}
	packet := dc.newPacket(layers.DHCPMsgTypeDiscover, xid, clientMac, dc.DHCPOptions)
	log.Printf("%s,sending %s:\n", dc.Iface.Name, layers.DHCPMsgTypeDiscover)
	dc.logger.PrintLog(packet)

	//start send
	go func () {
		defer lt.Stop()
		err = dc.sendMulticast(packet)
		if err != nil {
			log.Println(err)
			return
		}

		for resPacket := range c {
			msgType, lease := utility.NewLease(resPacket.(*layers.DHCPv4))
			log.Printf("[%s] received %s\n", dc.Iface.Name, msgType)
			dc.logger.PrintLog(resPacket)
			if request {
				err = dc.sendRequest(&lease, xid, clientMac)
				if err != nil {
					log.Println(err)
				}
			}
		}

		log.Println("discover over")

	}()

	return nil
}

func (dc *DhcpClient) sendRequest(lease *utility.Lease, xid uint32, clientMac net.HardwareAddr) error {
	// start listen thread
	lt := dc.getIdleListenThread()
	if lt == nil {
		return fmt.Errorf("no idle listen thread now\n")
	}
	c := make(chan interface{}, 10)
	lt.SetXid(xid)
	err := lt.Start(&dc.laddr, c, layers.DHCPMsgTypeAck, layers.DHCPMsgTypeNak)
	if err != nil {
		return err
	}

	//generate packet
	fixedAddress :=  []byte(lease.FixedAddress)
	serverID := []byte(lease.ServerID)

	packet := dc.newPacket(layers.DHCPMsgTypeRequest, xid, clientMac, append(dc.DHCPOptions,
		layers.NewDHCPOption(layers.DHCPOptRequestIP, fixedAddress),
		layers.NewDHCPOption(layers.DHCPOptServerID, serverID)))
	log.Printf("%s,sending %s:\n", dc.Iface.Name, layers.DHCPMsgTypeRequest)
	dc.logger.PrintLog(packet)

	//send request
	go func() {
		defer lt.Stop()
		err = dc.sendMulticast(packet)
		if err != nil {
			log.Println(err)
			return
		}

		for resPacket := range c {
			msgType, _ := utility.NewLease(resPacket.(*layers.DHCPv4))
			log.Printf("[%s] received %s\n", dc.Iface.Name, msgType)
			dc.logger.PrintLog(resPacket)
		}

		log.Println("request over")
	}()

	return nil

}



/*
func (dc *DhcpClient) discover() (*utility.Lease, error) {
	err := dc.sendPacket(layers.DHCPMsgTypeDiscover, dc.DHCPOptions)

	if err != nil {
		return nil, err
	}

	_, lease, err := dc.waitForResponse(layers.DHCPMsgTypeOffer)

	if err != nil {
		return nil, err
	}

	return lease, nil
}

func (dc *DhcpClient) waitForResponse(msgTypes ...layers.DHCPMsgType) (layers.DHCPMsgType, *utility.Lease, error) {
	con, err := dc.conn.Listen(&dc.laddr)

	if err != nil {
		return layers.DHCPMsgTypeUnspecified, nil, err
	}
	defer con.Close()

	err = con.SetReadDeadline(time.Now().Add(responseTimeout))

	if err != nil {
		return layers.DHCPMsgTypeUnspecified, nil, err
	}

	recvBuf := make([]byte, 342)

	for {

		_, _, err := con.ReadFrom(recvBuf)

		if err != nil {
			return layers.DHCPMsgTypeUnspecified, nil, err
		}

		packet := utility.ParsePacket(recvBuf)

		if packet == nil {
			continue
		}
		/*

		if packet.Xid == dc.xid && packet.Operation == layers.DHCPOpReply {
			msgType, res := utility.NewLease(packet)

			for _,t := range msgTypes {
				if t == msgType {
					log.Printf("[%s] received %s", dc.Iface.Name, msgType)
					dc.logger.PrintLog(packet)
					return msgType, &res, nil
				}
			}
		}
	}
}


/*
func (dc *DhcpClient) sendPacket(msgType layers.DHCPMsgType, options []layers.DHCPOption) error {
	packet := dc.newPacket(msgType, options)
	log.Printf("%s,sending %s:\n", dc.Iface.Name, msgType)
	dc.logger.PrintLog(packet)
	return dc.sendMulticast(packet)
}
*/

func (dc *DhcpClient) sendMulticast(dhcp *layers.DHCPv4) error {

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths: true,
	}

	err := gopacket.SerializeLayers(buf, opts, dhcp)

	if err != nil {
		return err
	}

    con, err := UDPDialer()(&dc.laddr, &net.UDPAddr{IP:net.IPv4bcast, Port:67})
	defer con.Close()

	con.SetWriteDeadline(time.Now().Add(dc.Timeout))

	_, err = con.Write(buf.Bytes())

	return err
}

func (dc *DhcpClient) newPacket(msgType layers.DHCPMsgType, xid uint32, clientMac net.HardwareAddr, options[]layers.DHCPOption) *layers.DHCPv4 {
	packet := layers.DHCPv4{
		Operation: layers.DHCPOpRequest,
		HardwareType: layers.LinkTypeEthernet,
		ClientHWAddr: clientMac,
		HardwareLen: uint8(len([]byte(dc.ClientMac))),
		Flags: uint16(layers.BroadcastFlag),
		ClientIP: net.ParseIP("0.0.0.0"),
		YourClientIP: net.ParseIP("0.0.0.0"),
		NextServerIP: net.ParseIP("0.0.0.0"),
		RelayAgentIP: net.ParseIP("0.0.0.0"),
		Xid: xid,
	}

	packet.Options = append(packet.Options, layers.NewDHCPOption(layers.DHCPOptMessageType, []byte{byte(msgType)}))
	for _, option := range options {
		packet.Options = append(packet.Options, layers.NewDHCPOption(option.Type, option.Data))
	}

	return &packet
}
