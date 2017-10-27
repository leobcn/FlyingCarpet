package main

import (
	"crypto/md5"
	"fmt"
	"github.com/dontpanic92/wxGo/wx"
	"math/rand"
	"net"
	// "os"
	// "os/user"
	"runtime"
	"strconv"
	"time"
)

const DIAL_TIMEOUT = 60
const JOIN_ADHOC_TIMEOUT = 60
const FIND_MAC_TIMEOUT = 60

// need different thread event for each post to output box, so need helper function that's receiver on Transfer?

func main() {
	wx1 := wx.NewApp()
	mf := newGui()
	mf.Show()
	wx1.MainLoop()
	return
}

func (t *Transfer) mainRoutine(mode string) {
	receiveChan := make(chan bool)
	sendChan := make(chan bool)
	var n Network

	if mode == "send" {
		pwBytes := md5.Sum([]byte(t.Passphrase))
		prefix := pwBytes[:3]
		t.SSID = fmt.Sprintf("flyingCarpet_%x", prefix)

		if runtime.GOOS == "windows" {
			w := WindowsNetwork{Mode: "sending"}
			w.PreviousSSID = w.getCurrentWifi()
			n = w
			t.output("Got current wifi")
		} else if runtime.GOOS == "darwin" {
			n = MacNetwork{Mode: "sending"}
		}
		if !n.connectToPeer(t) {
			t.enableStartButton()
			t.output("Exiting mainRoutine.")
			return
		}

		if connected := t.sendFile(sendChan, n); connected == false {
			t.enableStartButton()
			t.output("Could not establish TCP connection with peer. Exiting mainRoutine.")
			return
		}
		t.output("Connected")
		sendSuccess := <-sendChan
		if !sendSuccess {
			t.enableStartButton()
			t.output("Exiting mainRoutine.")
			return
		}
		t.enableStartButton()
		t.output("Send complete, resetting WiFi and exiting.")

	} else if mode == "receive" {
		t.Passphrase = generatePassword()
		pwBytes := md5.Sum([]byte(t.Passphrase))
		prefix := pwBytes[:3]
		t.SSID = fmt.Sprintf("flyingCarpet_%x", prefix)
		t.output(fmt.Sprintf("=============================\n"+
			"Transfer password: %s\nPlease use this password on sending end when prompted to start transfer.\n"+
			"=============================\n", t.Passphrase))

		if runtime.GOOS == "windows" {
			n = WindowsNetwork{Mode: "receiving"}
		} else if runtime.GOOS == "darwin" {
			n = MacNetwork{Mode: "receiving"}
		}
		if !n.connectToPeer(t) {
			t.enableStartButton()
			t.output("Exiting mainRoutine.")
			return
		}

		go t.receiveFile(receiveChan, n)
		// wait for listener to be up
		listenerIsUp := <-receiveChan
		if !listenerIsUp {
			t.enableStartButton()
			t.output("Exiting mainRoutine.")
			return
		}
		// wait for reception to finish
		receiveSuccess := <-receiveChan
		if !receiveSuccess {
			t.enableStartButton()
			t.output("Exiting mainRoutine.")
			return
		}
		t.output("Reception complete, resetting WiFi and exiting.")
	}
	n.resetWifi(t)
}

func (t *Transfer) receiveFile(receiveChan chan bool, n Network) {

	ln, err := net.Listen("tcp", ":"+strconv.Itoa(t.Port))
	if err != nil {
		n.teardown(t)
		t.output(fmt.Sprintf("Could not listen on :%d", t.Port))
		receiveChan <- false
		return
	}
	t.output("Listening on :" + strconv.Itoa(t.Port))
	receiveChan <- true
	for {
		conn, err := ln.Accept()
		if err != nil {
			n.teardown(t)
			t.output(fmt.Sprintf("Error accepting connection on :%d", t.Port))
			receiveChan <- false
			return
		}
		t.Conn = conn
		t.output("Connection accepted")
		go t.receiveAndAssemble(receiveChan, n)
	}
}

func (t *Transfer) sendFile(sendChan chan bool, n Network) bool {

	var conn net.Conn
	var err error
	for i := 0; i < DIAL_TIMEOUT; i++ {
		err = nil
		conn, err = net.DialTimeout("tcp", t.RecipientIP+":"+strconv.Itoa(t.Port), time.Millisecond*10)
		if err != nil {
			t.output(fmt.Sprintf("Failed connection %2d to %s, retrying.", i, t.RecipientIP))
			time.Sleep(time.Second * 1)
			continue
		}
		t.output("Successfully dialed peer.")
		t.Conn = conn
		go t.chunkAndSend(sendChan, n)
		return true
	}
	t.output(fmt.Sprintf("Waited %d seconds, no connection.", DIAL_TIMEOUT))
	return false
}

func generatePassword() string {
	const chars = "0123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ"
	rand.Seed(time.Now().UTC().UnixNano())
	pwBytes := make([]byte, 8)
	for i := range pwBytes {
		pwBytes[i] = chars[rand.Intn(len(chars))]
	}
	return string(pwBytes)
}

type Transfer struct {
	Filepath    string
	Passphrase  string
	SSID        string
	Conn        net.Conn
	Port        int
	RecipientIP string
	Peer        string
	AdHocChan   chan bool
	Frame       *MainFrame
}

type Network interface {
	connectToPeer(*Transfer) bool
	getCurrentWifi() string
	resetWifi(*Transfer)
	teardown(*Transfer)
}

type WindowsNetwork struct {
	Mode           string // sending or receiving
	PreviousSSID   string
	AdHocCapable   bool
	wifiDirectChan chan string
}

type MacNetwork struct {
	Mode string // sending or receiving
}
