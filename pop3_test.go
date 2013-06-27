package pop3

import (
	"log"
	"testing"
)

func TestNewPop3Client(*testing.T) {
	cli, err := NewClient("pop3.126.com:110", "test_pop3_golang", "test_pop3")
	if err != nil {
		log.Fatal(err)
	}
	defer cli.Close()
	list, err := cli.List()
	if err != nil {
		log.Fatal(err)
	}
	last := list[len(list)-1]
	msg, err := cli.GetMail(last.Id)
	if err != nil {
		log.Fatal(err)
	} else {
		if len(msg.RawMessage) != last.Size {
			log.Fatal("Get Email Size Is Not as expect")
		} else {
			log.Print(string(msg.UID))
		}
	}

}
