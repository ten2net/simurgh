// simurgh
// Copyright © 2016 Mike Tigas
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.
package main

import "bufio"
import "encoding/binary"
import "flag"
import "fmt"
import "math"
import "net"
import "os"
import "reflect"
import "sort"
import "time"

var MAGIC_MLAT_TIMESTAMP = []byte{0xFF, 0x00, 0x4D, 0x4C, 0x41, 0x54}

const AIS_CHARSET = "@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_ !\"#$%&'()*+,-./0123456789:;<=>?"


var listenAddr = flag.String("bind", "127.0.0.1:8081", "\":port\" or \"ip:port\" to bind the server to")
var baseLat = flag.Float64("baseLat", 40.77725, "latitude used for distance calculation")
var baseLon = flag.Float64("baseLon", -73.872611, "longitude for distance calculation")

const SORT_MODE_LASTPOS  = uint(0)
const SORT_MODE_DISTANCE = uint(1)
const SORT_MODE_CALLSIGN = uint(2)
var sortMode = flag.Uint("sortMode", SORT_MODE_DISTANCE, "0: sort by time, 1: sort by distance, 3: sort by air")



type Aircraft struct {
	icaoAddr uint32

	callsign string

	eRawLat uint32
	eRawLon uint32
	oRawLat uint32
	oRawLon uint32

	latitude  float64
	longitude float64
	altitude  int32

	lastPing time.Time
	lastPos  time.Time
}
type aircraftList []*Aircraft
type aircraftMap  map[uint32]*Aircraft


func main() {
	flag.Parse()


	// test: http://www.lll.lu/~edward/edward/adsb/DecodingADSBposition.html
	// parseRawLatLon(uint32(92095), uint32(39846), uint32(88385), uint32(125818), true, false)
	// test: http://wiki.modesbeast.com/Radarcape:Firmware_Versions#The_GPS_timestamp
	//timestamp := parseTime([]byte{0x24, 0x4b, 0xbb, 0x9a, 0xc9, 0xf0})
	//fmt.Println(timestamp)
	//os.Exit(1)


	fmt.Println("Launching server...")

	// listen on all interfaces
	ln, _ := net.Listen("tcp", *listenAddr)

	// accept connection on port
	conn, _ := ln.Accept()

	reader := bufio.NewReader(conn)

	var buffered_message []byte

	known_aircraft := make(aircraftMap)

	// run loop forever (or until ctrl-c)
	for {
		current_message, err := reader.ReadBytes(0x1A)
		if err != nil {
			fmt.Println("ERR:", err)
		}
		// Note `message` does not include 0x1A start byte b/c ReadBytes behavior

		if len(current_message) == 0 {
			break
		}

		// Add to our own buffer (or create it)
		if buffered_message == nil {
			buffered_message = current_message
		} else {
			buffered_message = append(buffered_message, current_message...)
		}

		// Are we on a *real* "message start" boundary? Then we're done
		// with our buffer.
		parseBuffer := false
		if current_message[0] == 0x31 || current_message[0] == 0x32 ||
			current_message[0] == 0x33 || current_message[0] == 0x34 {
			parseBuffer = true
		}

		if !parseBuffer {
			continue
		}
		message := buffered_message
		buffered_message = nil

		msgType := message[0]
		var msgLen int

		switch msgType {
		case 0x31:
			//fmt.Print("Type 1 Mode-AC")
			//msgLen = 10 // 2 + 8 header
			continue // not supported yet
		case 0x32:
			//fmt.Print("Type 2 Mode-S short")
			//msgLen = 15 // 7 + 8 header
			continue // later
		case 0x33:
			//fmt.Print("Type 3 Mode-S long")
			msgLen = 22 // 14
		case 0x34:
			//fmt.Print("Status Signal")
			//msgLen = 10 // ??
			continue // not supported
		default:
			continue
			//msgLen = 8 // shortest possible msg w/header & timetstamp
		}

		// Message wasn't long enough to contain the full header (maybe
		// input stream error), so skip
		if len(message) < msgLen {
			continue
		}

		fmt.Println()
		var timestamp time.Time
		if reflect.DeepEqual(message[1:7], MAGIC_MLAT_TIMESTAMP) {
		  //fmt.Println("FROM MLAT")
		  //otimestamp := parseTime(message[1:7])
		  //fmt.Println(otimestamp)
		  //timestamp = time.Now()
		} else {
		  timestamp = parseTime(message[1:7])
			_ = timestamp
			//fmt.Println(timestamp)
		}
		switch msgType {
		//case 0x31:
		//  fmt.Println("Type 1 Mode-AC")
		case 0x32:
			//fmt.Println("Type 2 Mode-S short")
		case 0x33:
			//fmt.Println("Type 3 Mode-S long")
			//case 0x34:
			//  fmt.Println("Status Signal")
		}

		//sigLevel := message[7]
		//fmt.Printf("Signal: %#02x (%d)\n", sigLevel, sigLevel)

		msgContent := message[8 : len(message)-1]
		////fmt.Printf("%d byte frame\n", len(msgContent))
		//for i:= 0; i < len(msgContent); i++ {
		//  fmt.Printf("%02x", msgContent[i])
		//}
		//fmt.Println()

		parseModeS(msgContent, &known_aircraft)
		//fmt.Println()

		printAircraftTable(&known_aircraft)
	}

}

func (a aircraftList) Len() int {
	return len(a)
}
func (a aircraftList) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}


func sortByDistance(a aircraftList, i, j int) bool {
	dist_i := greatcircle(a[i].latitude, a[i].longitude,
		*baseLat, *baseLon)
	dist_j := greatcircle(a[j].latitude, a[j].longitude,
		*baseLat, *baseLon)
	return dist_i < dist_j
}
func sortByCallsign(a aircraftList, i, j int) bool {
	if a[i].callsign != "" && a[j].callsign != "" {
		return a[i].callsign < a[j].callsign
	} else if a[i].callsign != "" && a[j].callsign == "" {
		return true
	} else if a[i].callsign == "" && a[j].callsign != "" {
		return false
	} else {
		hexi := fmt.Sprintf("%06x", a[i].icaoAddr)
		hexj := fmt.Sprintf("%06x", a[j].icaoAddr)
		return hexi < hexj
	}
}


func (a aircraftList) Less(i, j int) bool {
	if *sortMode == SORT_MODE_LASTPOS {
		// t1 later than t2 means that t1 is more recent
		return a[i].lastPos.After(a[j].lastPos)

	} else if *sortMode == SORT_MODE_DISTANCE {
		if (a[i].latitude != math.MaxFloat64 && a[j].latitude != math.MaxFloat64) {
			return sortByDistance(a, i, j)
		} else if (a[i].latitude != math.MaxFloat64 && a[j].latitude == math.MaxFloat64) {
			return true
		} else if (a[i].latitude == math.MaxFloat64 && a[j].latitude != math.MaxFloat64) {
			return false
		} else {
			return sortByCallsign(a, i, j)
		}
	} else if *sortMode == SORT_MODE_CALLSIGN {
		return sortByCallsign(a, i, j)
	} else {
		// ?
		//return a[i].lastPos > a[j].lastPos
	}
	return false
}


func printAircraftTable(known_aircraft *aircraftMap) {

	sortedAircraft := make(aircraftList, 0, len(*known_aircraft))

	for _, aircraft := range (*known_aircraft) {
		sortedAircraft = append(sortedAircraft, aircraft)
	}

	sort.Sort(sortedAircraft)

	for _, aircraft := range sortedAircraft {
		if time.Since(aircraft.lastPos) > (time.Duration(45) * time.Second) {
			continue
		}
		stale := (time.Since(aircraft.lastPos) > (time.Duration(10) * time.Second))
		extraStale := (time.Since(aircraft.lastPos) > (time.Duration(20) * time.Second))

		aircraftHasLocation := (aircraft.latitude != math.MaxFloat64 &&
			aircraft.longitude != math.MaxFloat64)
		aircraftHasAltitude := aircraft.altitude != math.MaxInt32

		if !aircraftHasLocation {
			continue
		}

		if aircraft.callsign != "" || aircraftHasLocation || aircraftHasAltitude {
			var sLatLon string
			var sAlt string

			if aircraftHasLocation {
				sLatLon = fmt.Sprintf("%f,%f", aircraft.latitude, aircraft.longitude)
			} else {
				sLatLon = "---.------,---.------"
			}
			if aircraftHasAltitude {
				sAlt = fmt.Sprintf("%d", aircraft.altitude)
			} else {
				sAlt = "-----"
			}

			distance := greatcircle(aircraft.latitude, aircraft.longitude,
				*baseLat, *baseLon)

			//tPing := time.Since(aircraft.lastPing)
			tPos := time.Since(aircraft.lastPos)

			if !stale && !extraStale {
				fmt.Printf("%06x\t%8s\t%s\t%s\t%3.2f\n",
					aircraft.icaoAddr, aircraft.callsign,
					sLatLon, sAlt, mtoMi(distance))
			} else if stale && !extraStale{
				fmt.Printf("%06x\t%8s\t%s?\t%s\t%3.2f?\n",
					aircraft.icaoAddr, aircraft.callsign,
					sLatLon, sAlt, mtoMi(distance))
			} else if extraStale {
				fmt.Printf("%06x\t%8s\t%s?\t%s\t%3.2f?\t%s…\n",
					aircraft.icaoAddr, aircraft.callsign,
					sLatLon, sAlt, mtoMi(distance),
					durationSecondsElapsed(tPos))
			}
		}
	}
	fmt.Println()
}

func durationSecondsElapsed(since time.Duration) string {
	sec := uint8(since.Seconds())
	if sec == math.MaxUint8 {
		return "-"
	} else {
		return fmt.Sprintf("%4d", sec)
	}

}

func parseModeS(message []byte, known_aircraft *aircraftMap) {
	// https://en.wikipedia.org/wiki/Secondary_surveillance_radar#Mode_S
	// https://github.com/mutability/dump1090/blob/master/mode_s.c
	linkFmt := uint((message[0] & 0xF8) >> 3)

	var aircraft Aircraft
	var aircraft_exists bool
	icaoAddr := uint32(math.MaxUint32)
	altCode := uint16(math.MaxUint16)
	altitude := int32(math.MaxInt32)

	//var msgType string
	//switch linkFmt {
	//case 0:
	//  msgType = "short air-air surveillance (TCAS)"
	//case 4:
	//  msgType = "surveillance, altitude reply"
	//case 5:
	//  msgType = "surveillance, Mode A identity reply"
	//case 11:
	//  msgType = "All-Call reply containing aircraft address"
	//case 16:
	//  msgType = "long air-air surveillance (TCAS)"
	//case 17:
	//  msgType = "extended squitter"
	//case 18:
	//  msgType = "TIS-B"
	//case 19:
	//  msgType = "military extended squitter"
	//case 20:
	//  msgType = "Comm-B including altitude reply"
	//case 21:
	//  msgType = "Comm-B reply including Mode A identity"
	//case 22:
	//  msgType = "military use"
	//case 24:
	//  msgType = "special long msg"
	//default:
	//  msgType = "unknown"
	//}
	//fmt.Printf("UF: %d\n", linkFmt)
	//fmt.Printf("UF: %08s\n", strconv.FormatInt(linkFmt, 2))
	//fmt.Println(msgType)

	if linkFmt == 11 || linkFmt == 17 || linkFmt == 18 {
		icaoAddr = uint32(message[1])*65536 + uint32(message[2])*256 + uint32(message[3])
		//fmt.Printf("ICAO: %06x\n", icaoAddr)
	}

	if icaoAddr != math.MaxUint32 {
		var ptrAircraft *Aircraft
		ptrAircraft, aircraft_exists = (*known_aircraft)[icaoAddr]
		if !aircraft_exists {
			// initialize some values
			aircraft = Aircraft{
				icaoAddr: icaoAddr,
				oRawLat: math.MaxUint32,
				oRawLon: math.MaxUint32,
				eRawLat: math.MaxUint32,
				eRawLon: math.MaxUint32,
				latitude: math.MaxFloat64,
				longitude: math.MaxFloat64,
				altitude: math.MaxInt32,
			  callsign: ""}
		} else {
			aircraft = (*ptrAircraft)
		}
		aircraft.lastPing = time.Now()
	}
	//fmt.Println(aircraft)
	//fmt.Println(aircraft_exists)

	if linkFmt == 0 || linkFmt == 4 || linkFmt == 16 || linkFmt == 20 {
		// Altitude: 13 bit signal
		altCode = (uint16(message[2])*256 + uint16(message[3])) & 0x1FFF

		if (altCode & 0x0040) > 0 {
			// meters
			// TODO
			fmt.Println("meters")

		} else if (altCode & 0x0010) > 0 {
			// feet, raw integer
			ac := (altCode&0x1F80)>>2 + (altCode&0x0020)>>1 + (altCode & 0x000F)
			altitude = int32((ac * 25) - 1000)
			// TODO
			//fmt.Println("int altitude: ", altitude)

		} else if (altCode & 0x0010) == 0 {
			// feet, Gillham coded
			// TODO
			fmt.Println("gillham")
		}

		if altitude != math.MaxInt32 {
			aircraft.altitude = altitude
		}
	}

	if linkFmt == 17 || linkFmt == 18 {
		decodeExtendedSquitter(message, linkFmt, &aircraft)
	}

	if icaoAddr != math.MaxUint32 {
		(*known_aircraft)[icaoAddr] = &aircraft
	}
	//fmt.Println(aircraft)
}

func parseTime(timebytes []byte) time.Time {
	// Takes a 6 byte array, which represents a 48bit GPS timestamp
	// http://wiki.modesbeast.com/Radarcape:Firmware_Versions#The_GPS_timestamp
	// and parses it into a Time.time

	upper := []byte{
		timebytes[0]<<2 + timebytes[1]>>6,
		timebytes[1]<<2 + timebytes[2]>>6,
		0, 0, 0, 0}
	lower := []byte{
		timebytes[2] & 0x3F, timebytes[3], timebytes[4], timebytes[5]}

	// the 48bit timestamp is 18bit day seconds | 30bit nanoseconds
	daySeconds := binary.BigEndian.Uint16(upper)
	nanoSeconds := int(binary.BigEndian.Uint32(lower))

	hr :=  int(daySeconds / 3600)
	min := int(daySeconds / 60 % 60)
	sec := int(daySeconds % 60)

	utcDate := time.Now().UTC()

	return time.Date(
		utcDate.Year(), utcDate.Month(), utcDate.Day(),
		hr, min, sec, nanoSeconds, time.UTC)
}

func decodeExtendedSquitter(message []byte, linkFmt uint,
	aircraft *Aircraft) {

	var callsign string

	//if linkFmt == 18 {
	//  switch (message[0] & 7) {
	//  case 1:
	//    fmt.Println("Non-ICAO")
	//  case 2:
	//    fmt.Println("TIS-B fine")
	//  case 3:
	//    fmt.Println("TIS-B coarse")
	//  case 5:
	//    fmt.Println("TIS-B anon ADS-B relay")
	//  case 6:
	//    fmt.Println("ADS-B rebroadcast")
	//  default:
	//    fmt.Println("Non-ICAO unknown")
	//  }
	//}

	msgType := uint(message[4]) >> 3
	var msgSubType uint
	if msgType == 29 {
		msgSubType = (uint(message[4]) & 6) >> 1
	} else {
		msgSubType = uint(message[4]) & 7
	}

	//fmt.Printf("ext msg: %d\n", msgType)

	raw_latitude := uint32(math.MaxUint32)
	raw_longitude := uint32(math.MaxUint32)
	latitude := float64(math.MaxFloat64)
	longitude := float64(math.MaxFloat64)
	altitude := int32(math.MaxInt32)

	switch msgType {
	case 1, 2, 3, 4:
		// Aircraft ID
		chars1 := uint(message[5])<<16 + uint(message[6])<<8 + uint(message[7])
		chars2 := uint(message[8])<<16 + uint(message[9])<<8 + uint(message[10])

		var fltByte [8]byte

		if chars1 != 0 && chars2 != 0 {
			// Flush the buffered raw bits into the representative 8 char string

			fltByte[3] = AIS_CHARSET[chars1&0x3F]
			chars1 >>= 6

			fltByte[2] = AIS_CHARSET[chars1&0x3F]
			chars1 >>= 6

			fltByte[1] = AIS_CHARSET[chars1&0x3F]
			chars1 >>= 6

			fltByte[0] = AIS_CHARSET[chars1&0x3F]

			fltByte[7] = AIS_CHARSET[chars2&0x3F]
			chars2 >>= 6

			fltByte[6] = AIS_CHARSET[chars2&0x3F]
			chars2 >>= 6

			fltByte[5] = AIS_CHARSET[chars2&0x3F]
			chars2 >>= 6

			fltByte[4] = AIS_CHARSET[chars2&0x3F]

			callsign = string(fltByte[:8])
			//fmt.Println("Callsign: ", callsign)
		}

	//case 19:
	//  // Airborne Velocity

	case 5, 6, 7, 8:
		// Ground position
		raw_latitude = uint32(message[6])&3<<15 + uint32(message[7])<<7 +
			uint32(message[8])>>1
		raw_longitude = uint32(message[8])&1<<16 + uint32(message[9])<<8 +
			uint32(message[10])

	case 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 20, 21, 22:
		// Airborne position

		ac12Data := (uint(message[5]) << 4) + (uint(message[6]) >> 4) & 0x0FFF
		if msgType != 0 {
			raw_latitude = uint32(message[6])&3<<15 + uint32(message[7])<<7 +
				uint32(message[8])>>1
			raw_longitude = uint32(message[8])&1<<16 + uint32(message[9])<<8 +
				uint32(message[10])
		}
		if msgType != 20 && msgType != 21 && msgType != 22 {
			//altitude :=
			//fmt.Printf("ac12: %#04x\n", ac12Data)
			//fmt.Printf("ac12: %d\n", decodeAC12Field(ac12Data))

			altitude = decodeAC12Field(ac12Data)

		} else {
			// "HAE" ac2-encoded altitude
			// TODO
		}
	}

	if (raw_latitude != math.MaxUint32) && (raw_longitude != math.MaxUint32) {
		tFlag := (byte(message[6]) & 8) == 8
		isOddFrame := (byte(message[6]) & 4) == 4

		if isOddFrame && aircraft.eRawLat != math.MaxUint32 && aircraft.eRawLon != math.MaxUint32 {
			// Odd frame and we have previous even frame data
			latitude, longitude = parseRawLatLon(aircraft.eRawLat, aircraft.eRawLon, raw_latitude, raw_longitude, isOddFrame, tFlag)
			// Reset our buffer
			aircraft.eRawLat = math.MaxUint32
			aircraft.eRawLon = math.MaxUint32
		} else if !isOddFrame && aircraft.oRawLat != math.MaxUint32 && aircraft.oRawLon != math.MaxUint32 {
			// Even frame and we have previous odd frame data
			latitude, longitude = parseRawLatLon(raw_latitude, raw_longitude, aircraft.oRawLat, aircraft.oRawLon, isOddFrame, tFlag)
			// Reset buffer
			aircraft.oRawLat = math.MaxUint32
			aircraft.oRawLon = math.MaxUint32
		} else if isOddFrame {
			aircraft.oRawLat = raw_latitude
			aircraft.oRawLon = raw_longitude
		} else if !isOddFrame {
			aircraft.eRawLat = raw_latitude
			aircraft.eRawLon = raw_longitude
		}
	}

	switch msgSubType {
	case 1:
		break
	case 99999:
		os.Exit(1)
	}

	if callsign != "" {
		aircraft.callsign = callsign
	}
	if altitude != math.MaxInt32 {
		aircraft.altitude = altitude
	}
	if latitude != math.MaxFloat64 && longitude != math.MaxFloat64 {
		aircraft.latitude = latitude
		aircraft.longitude = longitude
		aircraft.lastPos = time.Now()
	}
}

func parseRawLatLon(evenLat uint32, evenLon uint32, oddLat uint32,
	oddLon uint32, lastOdd bool, tFlag bool) (latitude float64, longitude float64) {
	if evenLat == math.MaxUint32 || oddLat == math.MaxUint32 ||
		oddLat == math.MaxUint32 || oddLon == math.MaxUint32 {
		return math.MaxFloat64, math.MaxFloat64
	}

	//fmt.Printf("Parsing: %d,%d + %d,%d\n", evenLat, evenLon, oddLat, oddLon)

	// http://www.lll.lu/~edward/edward/adsb/DecodingADSBposition.html
	j := int32((float64(59*evenLat-60*oddLat) / 131072.0) + 0.5)
	//fmt.Println("J: ", j)

	const airdlat0 = float64(6.0)
	const airdlat1 = float64(360.0) / float64(59.0)

	rlatEven := airdlat0 * (float64(j%60) + float64(evenLat)/131072.0)
	rlatOdd := airdlat1 * (float64(j%59) + float64(oddLat)/131072.0)
	if rlatEven >= 270 {
		rlatEven -= 360
	}
	if rlatOdd >= 270 {
		rlatOdd -= 360
	}

	//fmt.Println("rlat(0): ", rlatEven)
	//fmt.Println("rlat(1): ", rlatOdd)

	nlEven := cprNLFunction(rlatEven)
	nlOdd := cprNLFunction(rlatOdd)

	if nlEven != nlOdd {
		return math.MaxFloat64, math.MaxFloat64
	}

	//fmt.Println("NL(0): ", nlEven)
	//fmt.Println("NL(1): ", nlOdd)

	var ni int16

	if lastOdd {
		ni = int16(nlOdd) - 1
	} else {
		ni = int16(nlEven) - 1
	}
	if ni < 1 {
		ni = 1
	}
	//fmt.Println("NL(i): ", ni)

	//dlon := 360.0/float64(ni)
	//fmt.Println("dlon(i):", dlon)

	var m int16
	var outLat float64
	var outLon float64
	if tFlag {
		m = int16(math.Floor((float64(int32(evenLon*uint32(cprNLFunction(rlatOdd)-1))-
			int32(oddLon*uint32(cprNLFunction(rlatOdd)))) / 131072.0) + 0.5))
		outLon = cprDlonFunction(rlatOdd, tFlag, false) * (float64(m%ni) + float64(oddLon)/131072.0)
		outLat = rlatOdd

	} else {
		m = int16(math.Floor((float64(int32(evenLon*uint32(cprNLFunction(rlatEven)-1))-
			int32(oddLon*uint32(cprNLFunction(rlatEven)))) / 131072.0) + 0.5))
		outLon = cprDlonFunction(rlatEven, tFlag, false) * (float64(m%ni) + float64(evenLon)/131072.0)
		outLat = rlatEven
	}

	outLon -= math.Floor((outLon+180.0)/360.0) * 360.0

	//fmt.Println("M: ", m)
	//fmt.Println("outLat: ", outLat)
	//fmt.Println("outLon: ", outLon)

	return outLat, outLon
}


func cprNLFunction(lat float64) byte {
	if lat < 0 {
		lat = -lat
	}
	switch {
	case (lat < 10.47047130):
		return 59
	case (lat < 14.82817437):
		return 58
	case (lat < 18.18626357):
		return 57
	case (lat < 21.02939493):
		return 56
	case (lat < 23.54504487):
		return 55
	case (lat < 25.82924707):
		return 54
	case (lat < 27.93898710):
		return 53
	case (lat < 29.91135686):
		return 52
	case (lat < 31.77209708):
		return 51
	case (lat < 33.53993436):
		return 50
	case (lat < 35.22899598):
		return 49
	case (lat < 36.85025108):
		return 48
	case (lat < 38.41241892):
		return 47
	case (lat < 39.92256684):
		return 46
	case (lat < 41.38651832):
		return 45
	case (lat < 42.80914012):
		return 44
	case (lat < 44.19454951):
		return 43
	case (lat < 45.54626723):
		return 42
	case (lat < 46.86733252):
		return 41
	case (lat < 48.16039128):
		return 40
	case (lat < 49.42776439):
		return 39
	case (lat < 50.67150166):
		return 38
	case (lat < 51.89342469):
		return 37
	case (lat < 53.09516153):
		return 36
	case (lat < 54.27817472):
		return 35
	case (lat < 55.44378444):
		return 34
	case (lat < 56.59318756):
		return 33
	case (lat < 57.72747354):
		return 32
	case (lat < 58.84763776):
		return 31
	case (lat < 59.95459277):
		return 30
	case (lat < 61.04917774):
		return 29
	case (lat < 62.13216659):
		return 28
	case (lat < 63.20427479):
		return 27
	case (lat < 64.26616523):
		return 26
	case (lat < 65.31845310):
		return 25
	case (lat < 66.36171008):
		return 24
	case (lat < 67.39646774):
		return 23
	case (lat < 68.42322022):
		return 22
	case (lat < 69.44242631):
		return 21
	case (lat < 70.45451075):
		return 20
	case (lat < 71.45986473):
		return 19
	case (lat < 72.45884545):
		return 18
	case (lat < 73.45177442):
		return 17
	case (lat < 74.43893416):
		return 16
	case (lat < 75.42056257):
		return 15
	case (lat < 76.39684391):
		return 14
	case (lat < 77.36789461):
		return 13
	case (lat < 78.33374083):
		return 12
	case (lat < 79.29428225):
		return 11
	case (lat < 80.24923213):
		return 10
	case (lat < 81.19801349):
		return 9
	case (lat < 82.13956981):
		return 8
	case (lat < 83.07199445):
		return 7
	case (lat < 83.99173563):
		return 6
	case (lat < 84.89166191):
		return 5
	case (lat < 85.75541621):
		return 4
	case (lat < 86.53536998):
		return 3
	case (lat < 87.00000000):
		return 2
	default:
		return 1
	}
}
func cprNFunction(lat float64, fflag bool) byte {
	var t byte
	if fflag {
		t = 1
	} else {
		t = 0
	}

	nl := cprNLFunction(lat) - t
	if nl < 1 {
		nl = 1
	}
	return nl
}
func cprDlonFunction(lat float64, fflag bool, surface bool) float64 {
	var sfc float64
	if surface {
		sfc = 90.0
	} else {
		sfc = 360.0
	}

	return sfc / float64(cprNFunction(lat, fflag))

}


func decodeAC12Field(ac12Data uint) int32 {
	q := (ac12Data & 0x10) == 0x10
	if (q) {
		n := int32((ac12Data & 0x0FE0) >> 1) + int32(ac12Data & 0x000F)
		return (n*25) - 1000;
	} else {
		/* TODO
		// Make N a 13 bit Gillham coded altitude by inserting M=0 at bit 6
		int n = ((AC12Field & 0x0FC0) << 1) |
						 (AC12Field & 0x003F);
		n = ModeAToModeC(decodeID13Field(n));
		if (n < -12) {
				return INVALID_ALTITUDE;
		}

		return (100 * n);
		*/
		return int32(math.MaxInt32)
	}
}



func greatcircle(lat0, lon0, lat1, lon1 float64) float64{
    lat0 = lat0 * math.Pi / 180.0;
    lon0 = lon0 * math.Pi / 180.0;
    lat1 = lat1 * math.Pi / 180.0;
    lon1 = lon1 * math.Pi / 180.0;

    // avoid NaN
    if (math.Abs(lat0 - lat1) < 0.0001 && math.Abs(lon0 - lon1) < 0.0001) {
        return 0.0;
			}

    return 6371e3 * math.Acos(math.Sin(lat0) * math.Sin(lat1) + math.Cos(lat0) * math.Cos(lat1) * math.Cos(math.Abs(lon0 - lon1)));
}
func mtoMi(dist float64) float64 {
	return dist/float64(1609.34721869)
}
