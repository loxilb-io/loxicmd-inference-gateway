/*
 * Copyright (c) 2026 LoxiLB Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at:
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package api

import (
	"fmt"
	"strconv"
	"strings"
)

// BandTriple is one value for each priority band, written
// critical:standard:sheddable on the wire (the gateway's REST form for
// fc_band_weights, fc_band_min_share_pct, fc_band_wait_ms and
// fc_band_queue_pct: three whole numbers separated by ':', for example
// 8:3:1). ParseBandTriple and String are the only mapping between the two
// forms; JSON and YAML go through them.
type BandTriple struct {
	Critical  uint32
	Standard  uint32
	Sheddable uint32
}

// ParseBandTriple reads the wire form. It checks the shape only (three
// whole numbers separated by ':'); what the numbers may be is each
// field's rule and the gateway's to judge.
func ParseBandTriple(s string) (BandTriple, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return BandTriple{}, fmt.Errorf("%q is not three whole numbers separated by ':' (critical:standard:sheddable)", s)
	}
	var values [3]uint32
	for i, part := range parts {
		v, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return BandTriple{}, fmt.Errorf("%q is not three whole numbers separated by ':' (critical:standard:sheddable)", s)
		}
		values[i] = uint32(v)
	}
	return BandTriple{Critical: values[0], Standard: values[1], Sheddable: values[2]}, nil
}

// String writes the wire form, critical:standard:sheddable.
func (b BandTriple) String() string {
	return fmt.Sprintf("%d:%d:%d", b.Critical, b.Standard, b.Sheddable)
}

// MarshalText puts the wire form in JSON and YAML.
func (b BandTriple) MarshalText() ([]byte, error) {
	return []byte(b.String()), nil
}

// UnmarshalText reads the wire form from JSON and YAML.
func (b *BandTriple) UnmarshalText(text []byte) error {
	parsed, err := ParseBandTriple(string(text))
	if err != nil {
		return err
	}
	*b = parsed
	return nil
}
