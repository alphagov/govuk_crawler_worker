package main_test

import (
	"os"

	"github.com/fzzy/radix/redis"
	"github.com/golang/glog"
)

func DeleteMirrorFilesFromDisk(mirrorRoot string) {
	if mirrorRoot != "" {
		err := os.RemoveAll(mirrorRoot)
		if err != nil {
			glog.Errorln(err)
		}
	}
}

func PurgeAllKeys(prefix string, address string) error {
	client, err := redis.Dial("tcp", address)
	if err != nil {
		return err
	}

	keys, err := client.Cmd("KEYS", prefix+"*").List()
	if err != nil || len(keys) <= 0 {
		return err
	}

	reply := client.Cmd("DEL", keys)
	if reply.Err != nil {
		return reply.Err
	}

	return nil
}
