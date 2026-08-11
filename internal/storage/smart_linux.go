//go:build linux

package storage

import (
	"context"
	"encoding/json"
	"os/exec"
)

// querySmart shells out to smartctl in JSON mode. -n standby prevents waking
// spun-down drives; such drives simply keep their previous cached values.
func querySmart(ctx context.Context, device string) (smartResult, error) {
	cmd := exec.CommandContext(ctx, "smartctl", "-j", "-H", "-A", "-i", "-n", "standby", device)
	out, err := cmd.Output()
	// smartctl uses non-zero exit bits for non-fatal conditions (e.g. failing
	// health check = bit 3); parse whatever JSON we got regardless.
	if len(out) == 0 && err != nil {
		return smartResult{}, err
	}

	var doc struct {
		ModelName    string `json:"model_name"`
		SerialNumber string `json:"serial_number"`
		SmartStatus  *struct {
			Passed bool `json:"passed"`
		} `json:"smart_status"`
		Temperature struct {
			Current float64 `json:"current"`
		} `json:"temperature"`
		PowerOnTime struct {
			Hours int `json:"hours"`
		} `json:"power_on_time"`
		AtaAttrs struct {
			Table []struct {
				ID  int `json:"id"`
				Raw struct {
					Value int64 `json:"value"`
				} `json:"raw"`
			} `json:"table"`
		} `json:"ata_smart_attributes"`
		Nvme *struct {
			PercentageUsed  int `json:"percentage_used"`
			MediaErrors     int `json:"media_errors"`
			PowerOnHours    int `json:"power_on_hours"`
		} `json:"nvme_smart_health_information_log"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return smartResult{}, err
	}

	res := smartResult{
		model:  doc.ModelName,
		serial: doc.SerialNumber,
		tempC:  doc.Temperature.Current,
	}
	res.smart.Available = true
	if doc.SmartStatus != nil {
		healthy := doc.SmartStatus.Passed
		res.smart.Healthy = &healthy
	}
	res.smart.PowerOnHours = doc.PowerOnTime.Hours
	for _, a := range doc.AtaAttrs.Table {
		// Raw values of these attributes can pack multiple fields into high
		// bits; the low 16 bits are the count on all common drives.
		v := int(a.Raw.Value & 0xFFFF)
		switch a.ID {
		case 5:
			res.smart.Reallocated = v
		case 197:
			res.smart.PendingSectors = v
		case 199:
			res.smart.CRCErrors = v
		}
	}
	if doc.Nvme != nil {
		res.smart.PercentUsed = doc.Nvme.PercentageUsed
		res.smart.MediaErrors = doc.Nvme.MediaErrors
		if res.smart.PowerOnHours == 0 {
			res.smart.PowerOnHours = doc.Nvme.PowerOnHours
		}
	}
	return res, nil
}
