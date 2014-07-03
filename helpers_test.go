package main_test

import "log"
import "os"

func DeleteMirrorFilesFromDisk(mirrorRoot string) {
	if mirrorRoot != "" {
		err := os.RemoveAll(mirrorRoot)
		if err != nil {
			log.Println(err)
		}
	}
}
