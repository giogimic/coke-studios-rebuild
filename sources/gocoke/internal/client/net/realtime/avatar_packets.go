package realtime

import "encoding/xml"

// AvatarPacket is the shared wire boundary for both presence and action updates.
// Optional positions distinguish a spawn from an action-only AV packet.
type AvatarPacket struct {
	Name       string `xml:"av,attr"`
	Definition string `xml:"def,attr"`
	Mission    string `xml:"m,attr"`
	Gender     int    `xml:"g,attr"`
	Direction  int    `xml:"d,attr"`
	Action     string `xml:"act,attr"`
	X          *int   `xml:"x,attr"`
	Z          *int   `xml:"z,attr"`
}

// DecodeAvatarPackets decodes XML entities exactly once. Keep this independent
// of JS and graphics so server-to-client profile round trips are testable.
func DecodeAvatarPackets(packet []byte) ([]AvatarPacket, error) {
	var action struct {
		XMLName xml.Name       `xml:"Room.Action"`
		Avatars []AvatarPacket `xml:"AV"`
	}
	err := xml.Unmarshal(packet, &action)
	return action.Avatars, err
}
