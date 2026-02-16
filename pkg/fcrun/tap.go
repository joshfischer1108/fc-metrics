package fcrun

import "os/exec"

func createTap(name string) error {
	if err := exec.Command("ip", "tuntap", "add", "dev", name, "mode", "tap").Run(); err != nil {
		return err
	}
	if err := exec.Command("ip", "link", "set", "dev", name, "up").Run(); err != nil {
		_ = exec.Command("ip", "link", "del", "dev", name).Run()
		return err
	}
	return nil
}

func deleteTap(name string) {
	_ = exec.Command("ip", "link", "del", "dev", name).Run()
}

