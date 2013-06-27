package pop3

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"time"
)

type ListItem struct {
	Id   int
	Size int
}

type Mail struct {
	RawMessage []byte
	Message    *mail.Message
	UID        []byte
}

type Client struct {
	conn *net.TCPConn
}

func NewClient(addr string, username string, password string) (cli *Client, err error) {
	tcp_addr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return
	}
	conn, err := net.DialTCP("tcp", nil, tcp_addr)
	if err != nil {
		return
	}

	msg_buffer := make([]byte, 1024, 1024)

	conn.SetReadDeadline(time.Now().Add(1e9))
	defer func() {
		conn.SetReadDeadline(time.Time{})
	}()

	sz, err := conn.Read(msg_buffer)
	if err != nil {
		return
	} else if sz < 3 || string(msg_buffer[0:3]) != "+OK" {
		err = errors.New("Welcome Message Error")
		return
	}
	write_buffer := []byte(fmt.Sprintf("USER %s\r\n", username))

	sz, err = conn.Write(write_buffer)
	if err != nil {
		return
	} else if sz != len(write_buffer) {
		err = errors.New("Error while sending username")
		return
	}

	sz, err = conn.Read(msg_buffer)
	if err != nil {
		return
	} else if sz < 3 || string(msg_buffer[0:3]) != "+OK" {
		err = errors.New("Username is not exist " + string(msg_buffer[0:sz]))
		return
	}

	write_buffer = []byte(fmt.Sprintf("PASS %s\r\n", password))
	sz, err = conn.Write(write_buffer)
	if err != nil {
		return
	} else if sz != len(write_buffer) {
		err = errors.New("Error while sending password")
		return
	}

	sz, err = conn.Read(msg_buffer)
	if err != nil {
		return
	} else if sz < 3 || string(msg_buffer[0:3]) != "+OK" {
		err = errors.New("Password is not exist" + string(msg_buffer[0:sz]))
		return
	}

	cli = &Client{conn}

	return
}

func (c *Client) readPop3Message(timeout time.Duration) ([]byte, []byte, error) {
	t_out_chan := make(chan bool)
	go func() {
		time.Sleep(timeout)
		t_out_chan <- true
	}()

	read_buffer := make([]byte, 0)
	read_complete_chan := make(chan bool)
	read_error_chan := make(chan error)
	read_quit_chan := make(chan bool, 1)
	go func() {
		buf := make([]byte, 1024, 1024)
		for {
			select {
			case <-read_quit_chan:
				return
			default:
				sz, err := c.conn.Read(buf)
				if err != nil {
					read_error_chan <- err
					return
				}
				if sz != 0 {
					read_buffer = append(read_buffer, buf[:sz]...)
				}
				if read_buffer[0] != '+' {
					read_error_chan <- errors.New("Recieve Error " + string(read_buffer))
				} else if len(read_buffer) > 3 && string(read_buffer[len(read_buffer)-3:]) == ".\r\n" {
					read_complete_chan <- true
					return
				}
			}
		}
	}()

	select {
	case <-t_out_chan:
		read_quit_chan <- true
		return nil, read_buffer, errors.New("Read Time Out")
	case e := <-read_error_chan:
		return nil, read_buffer, e
	case <-read_complete_chan:
		header_buffer := read_buffer[:bytes.IndexByte(read_buffer, '\n')+1]
		read_buffer = read_buffer[bytes.IndexByte(read_buffer, '\n')+1:]
		read_buffer = read_buffer[:bytes.LastIndex(read_buffer, []byte{'.'})]
		return header_buffer, read_buffer, nil
	}

}

func (c *Client) List() ([]ListItem, error) {
	sz, err := c.conn.Write([]byte("LIST\r\n"))
	if err != nil {
		return nil, err
	} else if sz != 6 {
		return nil, errors.New("Error while sending list")
	}

	_, msg, err := c.readPop3Message(5e9)
	if err != nil {
		return nil, err
	}
	msg = msg[0 : len(msg)-2]
	lines := strings.Split(string(msg), "\r\n")
	retv := make([]ListItem, len(lines), len(lines))
	for i := range lines {
		l := strings.Split(lines[i], " ")
		retv[i].Id, err = strconv.Atoi(l[0])
		if err != nil {
			return retv, err
		}
		retv[i].Size, err = strconv.Atoi(l[1])
		if err != nil {
			return retv, err
		}
	}
	return retv, nil
}

func (c *Client) Delete(id int) error {
	write_msg := []byte(fmt.Sprintf("DELE %d\r\n", id))
	sz, err := c.conn.Write(write_msg)
	if err != nil {
		return err
	} else if sz != len(write_msg) {
		return errors.New("Write DELE Message error, Size Mismatch")
	}
	c.conn.SetReadDeadline(time.Now().Add(1e9))
	defer func() {
		c.conn.SetReadDeadline(time.Time{})
	}()
	buffer := make([]byte, 1024, 1024)
	sz, err = c.conn.Read(buffer)
	if err != nil {
		return err
	} else if sz == 0 || buffer[0] != '+' {
		return errors.New("Delete Option Error " + string(buffer[:sz]))
	} else {
		return nil
	}
}

func (c *Client) writeCommand(write_msg []byte) error {
	sz, err := c.conn.Write(write_msg)
	if err != nil {
		return err
	} else if sz != len(write_msg) {
		return errors.New("Write Message Error, Size Mismatch")
	} else {
		return err
	}
}

func (c *Client) GetUID(id int) ([]byte, error) {
	write_msg := []byte(fmt.Sprintf("UIDL %d\r\n", id))
	err := c.writeCommand(write_msg)
	if err != nil {
		return nil, err
	}
	buffer := make([]byte, 1024, 1024)
	t_out_chan := make(chan bool)
	read_ok_chan := make(chan bool)
	sz := 0
	go func() {
		time.Sleep(1e9)
		t_out_chan <- true
	}()
	go func() {
		sz, err = c.conn.Read(buffer)
		read_ok_chan <- true
	}()

	select {
	case <-t_out_chan:
		return nil, errors.New("Read Time Out")
	case <-read_ok_chan:
		buffer = buffer[:sz-2]
		buffer = buffer[bytes.LastIndex(buffer, []byte{' '})+1:]
		return buffer, nil
	}
}

func (c *Client) GetMail(id int) (*Mail, error) {
	raw_msg, err := c.get_mail_helper(id)
	if err != nil {
		return nil, err
	}
	uid, err := c.GetUID(id)
	if err != nil {
		return nil, err
	} else {
		msg, err := mail.ReadMessage(bytes.NewReader(raw_msg))
		if err != nil {
			return nil, err
		}

		return &Mail{raw_msg, msg, uid}, nil
	}
}

func (c *Client) get_mail_helper(id int) ([]byte, error) {
	write_msg := []byte(fmt.Sprintf("RETR %d\r\n", id))
	err := c.writeCommand(write_msg)
	if err != nil {
		return nil, err
	}
	head_buf, retv, err := c.readPop3Message(30e9)
	if err != nil {
		return nil, err
	} else {
		items := strings.Split(string(head_buf), " ")
		if len(items) != 3 {
			return retv, nil
		} else {
			sz, err := strconv.Atoi(items[1])
			if err != nil || sz >= len(retv) {
				return retv, nil
			} else {
				return retv[:sz], nil
			}
		}

	}
}

func (c *Client) Close() {
	c.conn.Write([]byte("QUIT\r\n"))
	defer c.conn.Close()
}
