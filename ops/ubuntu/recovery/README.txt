Hominal System Recovery

This USB contains SystemRescue and a recovery command bound to the single
ubuntu-vg/system-baseline snapshot on this machine's internal SSD.

Boot this USB in UEFI mode using the firmware's one-time boot menu. Do not make
the USB the permanent first boot device. SystemRescue copies itself to RAM,
verifies the internal LVM identity, and performs a read-only readiness check.

Check only:
  hominal-restore --check

Restore the root system, /boot, and EFI:
  hominal-restore --confirm-merge

The /agent logical volume is preserved. The restore command verifies the
machine, volume group, root LV, agent LV, and snapshot origin, and never runs
automatically.
