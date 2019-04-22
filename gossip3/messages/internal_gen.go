package messages

// Code generated by github.com/tinylib/msgp DO NOT EDIT.

import (
	"github.com/quorumcontrol/differencedigest/ibf"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/tinylib/msgp/msgp"
)

// DecodeMsg implements msgp.Decodable
func (z *DestinationHolder) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, err = dc.ReadMapHeader()
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "Destination":
			if dc.IsNil() {
				err = dc.ReadNil()
				if err != nil {
					err = msgp.WrapError(err, "Destination")
					return
				}
				z.Destination = nil
			} else {
				if z.Destination == nil {
					z.Destination = new(extmsgs.ActorPID)
				}
				err = z.Destination.DecodeMsg(dc)
				if err != nil {
					err = msgp.WrapError(err, "Destination")
					return
				}
			}
		default:
			err = dc.Skip()
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z *DestinationHolder) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 1
	// write "Destination"
	err = en.Append(0x81, 0xab, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e)
	if err != nil {
		return
	}
	if z.Destination == nil {
		err = en.WriteNil()
		if err != nil {
			return
		}
	} else {
		err = z.Destination.EncodeMsg(en)
		if err != nil {
			err = msgp.WrapError(err, "Destination")
			return
		}
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z *DestinationHolder) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 1
	// string "Destination"
	o = append(o, 0x81, 0xab, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e)
	if z.Destination == nil {
		o = msgp.AppendNil(o)
	} else {
		o, err = z.Destination.MarshalMsg(o)
		if err != nil {
			err = msgp.WrapError(err, "Destination")
			return
		}
	}
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *DestinationHolder) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "Destination":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				if err != nil {
					return
				}
				z.Destination = nil
			} else {
				if z.Destination == nil {
					z.Destination = new(extmsgs.ActorPID)
				}
				bts, err = z.Destination.UnmarshalMsg(bts)
				if err != nil {
					err = msgp.WrapError(err, "Destination")
					return
				}
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *DestinationHolder) Msgsize() (s int) {
	s = 1 + 12
	if z.Destination == nil {
		s += msgp.NilSize
	} else {
		s += z.Destination.Msgsize()
	}
	return
}

// DecodeMsg implements msgp.Decodable
func (z *GetSyncer) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, err = dc.ReadMapHeader()
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "Kind":
			z.Kind, err = dc.ReadString()
			if err != nil {
				err = msgp.WrapError(err, "Kind")
				return
			}
		default:
			err = dc.Skip()
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z GetSyncer) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 1
	// write "Kind"
	err = en.Append(0x81, 0xa4, 0x4b, 0x69, 0x6e, 0x64)
	if err != nil {
		return
	}
	err = en.WriteString(z.Kind)
	if err != nil {
		err = msgp.WrapError(err, "Kind")
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z GetSyncer) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 1
	// string "Kind"
	o = append(o, 0x81, 0xa4, 0x4b, 0x69, 0x6e, 0x64)
	o = msgp.AppendString(o, z.Kind)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *GetSyncer) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "Kind":
			z.Kind, bts, err = msgp.ReadStringBytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "Kind")
				return
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z GetSyncer) Msgsize() (s int) {
	s = 1 + 5 + msgp.StringPrefixSize + len(z.Kind)
	return
}

// DecodeMsg implements msgp.Decodable
func (z *NoSyncersAvailable) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, err = dc.ReadMapHeader()
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		default:
			err = dc.Skip()
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z NoSyncersAvailable) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 0
	err = en.Append(0x80)
	if err != nil {
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z NoSyncersAvailable) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 0
	o = append(o, 0x80)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *NoSyncersAvailable) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z NoSyncersAvailable) Msgsize() (s int) {
	s = 1
	return
}

// DecodeMsg implements msgp.Decodable
func (z *ProvideBloomFilter) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, err = dc.ReadMapHeader()
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "DestinationHolder":
			var zb0002 uint32
			zb0002, err = dc.ReadMapHeader()
			if err != nil {
				err = msgp.WrapError(err, "DestinationHolder")
				return
			}
			for zb0002 > 0 {
				zb0002--
				field, err = dc.ReadMapKeyPtr()
				if err != nil {
					err = msgp.WrapError(err, "DestinationHolder")
					return
				}
				switch msgp.UnsafeString(field) {
				case "Destination":
					if dc.IsNil() {
						err = dc.ReadNil()
						if err != nil {
							err = msgp.WrapError(err, "DestinationHolder", "Destination")
							return
						}
						z.DestinationHolder.Destination = nil
					} else {
						if z.DestinationHolder.Destination == nil {
							z.DestinationHolder.Destination = new(extmsgs.ActorPID)
						}
						err = z.DestinationHolder.Destination.DecodeMsg(dc)
						if err != nil {
							err = msgp.WrapError(err, "DestinationHolder", "Destination")
							return
						}
					}
				default:
					err = dc.Skip()
					if err != nil {
						err = msgp.WrapError(err, "DestinationHolder")
						return
					}
				}
			}
		case "Filter":
			if dc.IsNil() {
				err = dc.ReadNil()
				if err != nil {
					err = msgp.WrapError(err, "Filter")
					return
				}
				z.Filter = nil
			} else {
				if z.Filter == nil {
					z.Filter = new(ibf.InvertibleBloomFilter)
				}
				err = z.Filter.DecodeMsg(dc)
				if err != nil {
					err = msgp.WrapError(err, "Filter")
					return
				}
			}
		default:
			err = dc.Skip()
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z *ProvideBloomFilter) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 2
	// write "DestinationHolder"
	// map header, size 1
	// write "Destination"
	err = en.Append(0x82, 0xb1, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x48, 0x6f, 0x6c, 0x64, 0x65, 0x72, 0x81, 0xab, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e)
	if err != nil {
		return
	}
	if z.DestinationHolder.Destination == nil {
		err = en.WriteNil()
		if err != nil {
			return
		}
	} else {
		err = z.DestinationHolder.Destination.EncodeMsg(en)
		if err != nil {
			err = msgp.WrapError(err, "DestinationHolder", "Destination")
			return
		}
	}
	// write "Filter"
	err = en.Append(0xa6, 0x46, 0x69, 0x6c, 0x74, 0x65, 0x72)
	if err != nil {
		return
	}
	if z.Filter == nil {
		err = en.WriteNil()
		if err != nil {
			return
		}
	} else {
		err = z.Filter.EncodeMsg(en)
		if err != nil {
			err = msgp.WrapError(err, "Filter")
			return
		}
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z *ProvideBloomFilter) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 2
	// string "DestinationHolder"
	// map header, size 1
	// string "Destination"
	o = append(o, 0x82, 0xb1, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x48, 0x6f, 0x6c, 0x64, 0x65, 0x72, 0x81, 0xab, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e)
	if z.DestinationHolder.Destination == nil {
		o = msgp.AppendNil(o)
	} else {
		o, err = z.DestinationHolder.Destination.MarshalMsg(o)
		if err != nil {
			err = msgp.WrapError(err, "DestinationHolder", "Destination")
			return
		}
	}
	// string "Filter"
	o = append(o, 0xa6, 0x46, 0x69, 0x6c, 0x74, 0x65, 0x72)
	if z.Filter == nil {
		o = msgp.AppendNil(o)
	} else {
		o, err = z.Filter.MarshalMsg(o)
		if err != nil {
			err = msgp.WrapError(err, "Filter")
			return
		}
	}
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *ProvideBloomFilter) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "DestinationHolder":
			var zb0002 uint32
			zb0002, bts, err = msgp.ReadMapHeaderBytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "DestinationHolder")
				return
			}
			for zb0002 > 0 {
				zb0002--
				field, bts, err = msgp.ReadMapKeyZC(bts)
				if err != nil {
					err = msgp.WrapError(err, "DestinationHolder")
					return
				}
				switch msgp.UnsafeString(field) {
				case "Destination":
					if msgp.IsNil(bts) {
						bts, err = msgp.ReadNilBytes(bts)
						if err != nil {
							return
						}
						z.DestinationHolder.Destination = nil
					} else {
						if z.DestinationHolder.Destination == nil {
							z.DestinationHolder.Destination = new(extmsgs.ActorPID)
						}
						bts, err = z.DestinationHolder.Destination.UnmarshalMsg(bts)
						if err != nil {
							err = msgp.WrapError(err, "DestinationHolder", "Destination")
							return
						}
					}
				default:
					bts, err = msgp.Skip(bts)
					if err != nil {
						err = msgp.WrapError(err, "DestinationHolder")
						return
					}
				}
			}
		case "Filter":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				if err != nil {
					return
				}
				z.Filter = nil
			} else {
				if z.Filter == nil {
					z.Filter = new(ibf.InvertibleBloomFilter)
				}
				bts, err = z.Filter.UnmarshalMsg(bts)
				if err != nil {
					err = msgp.WrapError(err, "Filter")
					return
				}
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *ProvideBloomFilter) Msgsize() (s int) {
	s = 1 + 18 + 1 + 12
	if z.DestinationHolder.Destination == nil {
		s += msgp.NilSize
	} else {
		s += z.DestinationHolder.Destination.Msgsize()
	}
	s += 7
	if z.Filter == nil {
		s += msgp.NilSize
	} else {
		s += z.Filter.Msgsize()
	}
	return
}

// DecodeMsg implements msgp.Decodable
func (z *ProvideStrata) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, err = dc.ReadMapHeader()
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "DestinationHolder":
			var zb0002 uint32
			zb0002, err = dc.ReadMapHeader()
			if err != nil {
				err = msgp.WrapError(err, "DestinationHolder")
				return
			}
			for zb0002 > 0 {
				zb0002--
				field, err = dc.ReadMapKeyPtr()
				if err != nil {
					err = msgp.WrapError(err, "DestinationHolder")
					return
				}
				switch msgp.UnsafeString(field) {
				case "Destination":
					if dc.IsNil() {
						err = dc.ReadNil()
						if err != nil {
							err = msgp.WrapError(err, "DestinationHolder", "Destination")
							return
						}
						z.DestinationHolder.Destination = nil
					} else {
						if z.DestinationHolder.Destination == nil {
							z.DestinationHolder.Destination = new(extmsgs.ActorPID)
						}
						err = z.DestinationHolder.Destination.DecodeMsg(dc)
						if err != nil {
							err = msgp.WrapError(err, "DestinationHolder", "Destination")
							return
						}
					}
				default:
					err = dc.Skip()
					if err != nil {
						err = msgp.WrapError(err, "DestinationHolder")
						return
					}
				}
			}
		case "Strata":
			if dc.IsNil() {
				err = dc.ReadNil()
				if err != nil {
					err = msgp.WrapError(err, "Strata")
					return
				}
				z.Strata = nil
			} else {
				if z.Strata == nil {
					z.Strata = new(ibf.DifferenceStrata)
				}
				err = z.Strata.DecodeMsg(dc)
				if err != nil {
					err = msgp.WrapError(err, "Strata")
					return
				}
			}
		default:
			err = dc.Skip()
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z *ProvideStrata) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 2
	// write "DestinationHolder"
	// map header, size 1
	// write "Destination"
	err = en.Append(0x82, 0xb1, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x48, 0x6f, 0x6c, 0x64, 0x65, 0x72, 0x81, 0xab, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e)
	if err != nil {
		return
	}
	if z.DestinationHolder.Destination == nil {
		err = en.WriteNil()
		if err != nil {
			return
		}
	} else {
		err = z.DestinationHolder.Destination.EncodeMsg(en)
		if err != nil {
			err = msgp.WrapError(err, "DestinationHolder", "Destination")
			return
		}
	}
	// write "Strata"
	err = en.Append(0xa6, 0x53, 0x74, 0x72, 0x61, 0x74, 0x61)
	if err != nil {
		return
	}
	if z.Strata == nil {
		err = en.WriteNil()
		if err != nil {
			return
		}
	} else {
		err = z.Strata.EncodeMsg(en)
		if err != nil {
			err = msgp.WrapError(err, "Strata")
			return
		}
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z *ProvideStrata) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 2
	// string "DestinationHolder"
	// map header, size 1
	// string "Destination"
	o = append(o, 0x82, 0xb1, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x48, 0x6f, 0x6c, 0x64, 0x65, 0x72, 0x81, 0xab, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e)
	if z.DestinationHolder.Destination == nil {
		o = msgp.AppendNil(o)
	} else {
		o, err = z.DestinationHolder.Destination.MarshalMsg(o)
		if err != nil {
			err = msgp.WrapError(err, "DestinationHolder", "Destination")
			return
		}
	}
	// string "Strata"
	o = append(o, 0xa6, 0x53, 0x74, 0x72, 0x61, 0x74, 0x61)
	if z.Strata == nil {
		o = msgp.AppendNil(o)
	} else {
		o, err = z.Strata.MarshalMsg(o)
		if err != nil {
			err = msgp.WrapError(err, "Strata")
			return
		}
	}
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *ProvideStrata) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "DestinationHolder":
			var zb0002 uint32
			zb0002, bts, err = msgp.ReadMapHeaderBytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "DestinationHolder")
				return
			}
			for zb0002 > 0 {
				zb0002--
				field, bts, err = msgp.ReadMapKeyZC(bts)
				if err != nil {
					err = msgp.WrapError(err, "DestinationHolder")
					return
				}
				switch msgp.UnsafeString(field) {
				case "Destination":
					if msgp.IsNil(bts) {
						bts, err = msgp.ReadNilBytes(bts)
						if err != nil {
							return
						}
						z.DestinationHolder.Destination = nil
					} else {
						if z.DestinationHolder.Destination == nil {
							z.DestinationHolder.Destination = new(extmsgs.ActorPID)
						}
						bts, err = z.DestinationHolder.Destination.UnmarshalMsg(bts)
						if err != nil {
							err = msgp.WrapError(err, "DestinationHolder", "Destination")
							return
						}
					}
				default:
					bts, err = msgp.Skip(bts)
					if err != nil {
						err = msgp.WrapError(err, "DestinationHolder")
						return
					}
				}
			}
		case "Strata":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				if err != nil {
					return
				}
				z.Strata = nil
			} else {
				if z.Strata == nil {
					z.Strata = new(ibf.DifferenceStrata)
				}
				bts, err = z.Strata.UnmarshalMsg(bts)
				if err != nil {
					err = msgp.WrapError(err, "Strata")
					return
				}
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *ProvideStrata) Msgsize() (s int) {
	s = 1 + 18 + 1 + 12
	if z.DestinationHolder.Destination == nil {
		s += msgp.NilSize
	} else {
		s += z.DestinationHolder.Destination.Msgsize()
	}
	s += 7
	if z.Strata == nil {
		s += msgp.NilSize
	} else {
		s += z.Strata.Msgsize()
	}
	return
}

// DecodeMsg implements msgp.Decodable
func (z *ReceiveFullExchange) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, err = dc.ReadMapHeader()
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "Payload":
			z.Payload, err = dc.ReadBytes(z.Payload)
			if err != nil {
				err = msgp.WrapError(err, "Payload")
				return
			}
		case "RequestExchangeBack":
			z.RequestExchangeBack, err = dc.ReadBool()
			if err != nil {
				err = msgp.WrapError(err, "RequestExchangeBack")
				return
			}
		default:
			err = dc.Skip()
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z *ReceiveFullExchange) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 2
	// write "Payload"
	err = en.Append(0x82, 0xa7, 0x50, 0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64)
	if err != nil {
		return
	}
	err = en.WriteBytes(z.Payload)
	if err != nil {
		err = msgp.WrapError(err, "Payload")
		return
	}
	// write "RequestExchangeBack"
	err = en.Append(0xb3, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x45, 0x78, 0x63, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x42, 0x61, 0x63, 0x6b)
	if err != nil {
		return
	}
	err = en.WriteBool(z.RequestExchangeBack)
	if err != nil {
		err = msgp.WrapError(err, "RequestExchangeBack")
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z *ReceiveFullExchange) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 2
	// string "Payload"
	o = append(o, 0x82, 0xa7, 0x50, 0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64)
	o = msgp.AppendBytes(o, z.Payload)
	// string "RequestExchangeBack"
	o = append(o, 0xb3, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x45, 0x78, 0x63, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x42, 0x61, 0x63, 0x6b)
	o = msgp.AppendBool(o, z.RequestExchangeBack)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *ReceiveFullExchange) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "Payload":
			z.Payload, bts, err = msgp.ReadBytesBytes(bts, z.Payload)
			if err != nil {
				err = msgp.WrapError(err, "Payload")
				return
			}
		case "RequestExchangeBack":
			z.RequestExchangeBack, bts, err = msgp.ReadBoolBytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "RequestExchangeBack")
				return
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *ReceiveFullExchange) Msgsize() (s int) {
	s = 1 + 8 + msgp.BytesPrefixSize + len(z.Payload) + 20 + msgp.BoolSize
	return
}

// DecodeMsg implements msgp.Decodable
func (z *RequestFullExchange) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, err = dc.ReadMapHeader()
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "DestinationHolder":
			var zb0002 uint32
			zb0002, err = dc.ReadMapHeader()
			if err != nil {
				err = msgp.WrapError(err, "DestinationHolder")
				return
			}
			for zb0002 > 0 {
				zb0002--
				field, err = dc.ReadMapKeyPtr()
				if err != nil {
					err = msgp.WrapError(err, "DestinationHolder")
					return
				}
				switch msgp.UnsafeString(field) {
				case "Destination":
					if dc.IsNil() {
						err = dc.ReadNil()
						if err != nil {
							err = msgp.WrapError(err, "DestinationHolder", "Destination")
							return
						}
						z.DestinationHolder.Destination = nil
					} else {
						if z.DestinationHolder.Destination == nil {
							z.DestinationHolder.Destination = new(extmsgs.ActorPID)
						}
						err = z.DestinationHolder.Destination.DecodeMsg(dc)
						if err != nil {
							err = msgp.WrapError(err, "DestinationHolder", "Destination")
							return
						}
					}
				default:
					err = dc.Skip()
					if err != nil {
						err = msgp.WrapError(err, "DestinationHolder")
						return
					}
				}
			}
		default:
			err = dc.Skip()
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z *RequestFullExchange) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 1
	// write "DestinationHolder"
	// map header, size 1
	// write "Destination"
	err = en.Append(0x81, 0xb1, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x48, 0x6f, 0x6c, 0x64, 0x65, 0x72, 0x81, 0xab, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e)
	if err != nil {
		return
	}
	if z.DestinationHolder.Destination == nil {
		err = en.WriteNil()
		if err != nil {
			return
		}
	} else {
		err = z.DestinationHolder.Destination.EncodeMsg(en)
		if err != nil {
			err = msgp.WrapError(err, "DestinationHolder", "Destination")
			return
		}
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z *RequestFullExchange) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 1
	// string "DestinationHolder"
	// map header, size 1
	// string "Destination"
	o = append(o, 0x81, 0xb1, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x48, 0x6f, 0x6c, 0x64, 0x65, 0x72, 0x81, 0xab, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e)
	if z.DestinationHolder.Destination == nil {
		o = msgp.AppendNil(o)
	} else {
		o, err = z.DestinationHolder.Destination.MarshalMsg(o)
		if err != nil {
			err = msgp.WrapError(err, "DestinationHolder", "Destination")
			return
		}
	}
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *RequestFullExchange) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "DestinationHolder":
			var zb0002 uint32
			zb0002, bts, err = msgp.ReadMapHeaderBytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "DestinationHolder")
				return
			}
			for zb0002 > 0 {
				zb0002--
				field, bts, err = msgp.ReadMapKeyZC(bts)
				if err != nil {
					err = msgp.WrapError(err, "DestinationHolder")
					return
				}
				switch msgp.UnsafeString(field) {
				case "Destination":
					if msgp.IsNil(bts) {
						bts, err = msgp.ReadNilBytes(bts)
						if err != nil {
							return
						}
						z.DestinationHolder.Destination = nil
					} else {
						if z.DestinationHolder.Destination == nil {
							z.DestinationHolder.Destination = new(extmsgs.ActorPID)
						}
						bts, err = z.DestinationHolder.Destination.UnmarshalMsg(bts)
						if err != nil {
							err = msgp.WrapError(err, "DestinationHolder", "Destination")
							return
						}
					}
				default:
					bts, err = msgp.Skip(bts)
					if err != nil {
						err = msgp.WrapError(err, "DestinationHolder")
						return
					}
				}
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *RequestFullExchange) Msgsize() (s int) {
	s = 1 + 18 + 1 + 12
	if z.DestinationHolder.Destination == nil {
		s += msgp.NilSize
	} else {
		s += z.DestinationHolder.Destination.Msgsize()
	}
	return
}

// DecodeMsg implements msgp.Decodable
func (z *RequestIBF) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, err = dc.ReadMapHeader()
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "Count":
			z.Count, err = dc.ReadInt()
			if err != nil {
				err = msgp.WrapError(err, "Count")
				return
			}
		default:
			err = dc.Skip()
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z RequestIBF) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 1
	// write "Count"
	err = en.Append(0x81, 0xa5, 0x43, 0x6f, 0x75, 0x6e, 0x74)
	if err != nil {
		return
	}
	err = en.WriteInt(z.Count)
	if err != nil {
		err = msgp.WrapError(err, "Count")
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z RequestIBF) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 1
	// string "Count"
	o = append(o, 0x81, 0xa5, 0x43, 0x6f, 0x75, 0x6e, 0x74)
	o = msgp.AppendInt(o, z.Count)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *RequestIBF) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "Count":
			z.Count, bts, err = msgp.ReadIntBytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "Count")
				return
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z RequestIBF) Msgsize() (s int) {
	s = 1 + 6 + msgp.IntSize
	return
}

// DecodeMsg implements msgp.Decodable
func (z *RequestKeys) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, err = dc.ReadMapHeader()
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "Keys":
			var zb0002 uint32
			zb0002, err = dc.ReadArrayHeader()
			if err != nil {
				err = msgp.WrapError(err, "Keys")
				return
			}
			if cap(z.Keys) >= int(zb0002) {
				z.Keys = (z.Keys)[:zb0002]
			} else {
				z.Keys = make([]uint64, zb0002)
			}
			for za0001 := range z.Keys {
				z.Keys[za0001], err = dc.ReadUint64()
				if err != nil {
					err = msgp.WrapError(err, "Keys", za0001)
					return
				}
			}
		default:
			err = dc.Skip()
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z *RequestKeys) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 1
	// write "Keys"
	err = en.Append(0x81, 0xa4, 0x4b, 0x65, 0x79, 0x73)
	if err != nil {
		return
	}
	err = en.WriteArrayHeader(uint32(len(z.Keys)))
	if err != nil {
		err = msgp.WrapError(err, "Keys")
		return
	}
	for za0001 := range z.Keys {
		err = en.WriteUint64(z.Keys[za0001])
		if err != nil {
			err = msgp.WrapError(err, "Keys", za0001)
			return
		}
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z *RequestKeys) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 1
	// string "Keys"
	o = append(o, 0x81, 0xa4, 0x4b, 0x65, 0x79, 0x73)
	o = msgp.AppendArrayHeader(o, uint32(len(z.Keys)))
	for za0001 := range z.Keys {
		o = msgp.AppendUint64(o, z.Keys[za0001])
	}
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *RequestKeys) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "Keys":
			var zb0002 uint32
			zb0002, bts, err = msgp.ReadArrayHeaderBytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "Keys")
				return
			}
			if cap(z.Keys) >= int(zb0002) {
				z.Keys = (z.Keys)[:zb0002]
			} else {
				z.Keys = make([]uint64, zb0002)
			}
			for za0001 := range z.Keys {
				z.Keys[za0001], bts, err = msgp.ReadUint64Bytes(bts)
				if err != nil {
					err = msgp.WrapError(err, "Keys", za0001)
					return
				}
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *RequestKeys) Msgsize() (s int) {
	s = 1 + 5 + msgp.ArrayHeaderSize + (len(z.Keys) * (msgp.Uint64Size))
	return
}

// DecodeMsg implements msgp.Decodable
func (z *SyncDone) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, err = dc.ReadMapHeader()
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		default:
			err = dc.Skip()
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z SyncDone) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 0
	err = en.Append(0x80)
	if err != nil {
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z SyncDone) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 0
	o = append(o, 0x80)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *SyncDone) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z SyncDone) Msgsize() (s int) {
	s = 1
	return
}

// DecodeMsg implements msgp.Decodable
func (z *SyncerAvailable) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, err = dc.ReadMapHeader()
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "DestinationHolder":
			var zb0002 uint32
			zb0002, err = dc.ReadMapHeader()
			if err != nil {
				err = msgp.WrapError(err, "DestinationHolder")
				return
			}
			for zb0002 > 0 {
				zb0002--
				field, err = dc.ReadMapKeyPtr()
				if err != nil {
					err = msgp.WrapError(err, "DestinationHolder")
					return
				}
				switch msgp.UnsafeString(field) {
				case "Destination":
					if dc.IsNil() {
						err = dc.ReadNil()
						if err != nil {
							err = msgp.WrapError(err, "DestinationHolder", "Destination")
							return
						}
						z.DestinationHolder.Destination = nil
					} else {
						if z.DestinationHolder.Destination == nil {
							z.DestinationHolder.Destination = new(extmsgs.ActorPID)
						}
						err = z.DestinationHolder.Destination.DecodeMsg(dc)
						if err != nil {
							err = msgp.WrapError(err, "DestinationHolder", "Destination")
							return
						}
					}
				default:
					err = dc.Skip()
					if err != nil {
						err = msgp.WrapError(err, "DestinationHolder")
						return
					}
				}
			}
		default:
			err = dc.Skip()
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z *SyncerAvailable) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 1
	// write "DestinationHolder"
	// map header, size 1
	// write "Destination"
	err = en.Append(0x81, 0xb1, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x48, 0x6f, 0x6c, 0x64, 0x65, 0x72, 0x81, 0xab, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e)
	if err != nil {
		return
	}
	if z.DestinationHolder.Destination == nil {
		err = en.WriteNil()
		if err != nil {
			return
		}
	} else {
		err = z.DestinationHolder.Destination.EncodeMsg(en)
		if err != nil {
			err = msgp.WrapError(err, "DestinationHolder", "Destination")
			return
		}
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z *SyncerAvailable) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 1
	// string "DestinationHolder"
	// map header, size 1
	// string "Destination"
	o = append(o, 0x81, 0xb1, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x48, 0x6f, 0x6c, 0x64, 0x65, 0x72, 0x81, 0xab, 0x44, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x61, 0x74, 0x69, 0x6f, 0x6e)
	if z.DestinationHolder.Destination == nil {
		o = msgp.AppendNil(o)
	} else {
		o, err = z.DestinationHolder.Destination.MarshalMsg(o)
		if err != nil {
			err = msgp.WrapError(err, "DestinationHolder", "Destination")
			return
		}
	}
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *SyncerAvailable) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "DestinationHolder":
			var zb0002 uint32
			zb0002, bts, err = msgp.ReadMapHeaderBytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "DestinationHolder")
				return
			}
			for zb0002 > 0 {
				zb0002--
				field, bts, err = msgp.ReadMapKeyZC(bts)
				if err != nil {
					err = msgp.WrapError(err, "DestinationHolder")
					return
				}
				switch msgp.UnsafeString(field) {
				case "Destination":
					if msgp.IsNil(bts) {
						bts, err = msgp.ReadNilBytes(bts)
						if err != nil {
							return
						}
						z.DestinationHolder.Destination = nil
					} else {
						if z.DestinationHolder.Destination == nil {
							z.DestinationHolder.Destination = new(extmsgs.ActorPID)
						}
						bts, err = z.DestinationHolder.Destination.UnmarshalMsg(bts)
						if err != nil {
							err = msgp.WrapError(err, "DestinationHolder", "Destination")
							return
						}
					}
				default:
					bts, err = msgp.Skip(bts)
					if err != nil {
						err = msgp.WrapError(err, "DestinationHolder")
						return
					}
				}
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *SyncerAvailable) Msgsize() (s int) {
	s = 1 + 18 + 1 + 12
	if z.DestinationHolder.Destination == nil {
		s += msgp.NilSize
	} else {
		s += z.DestinationHolder.Destination.Msgsize()
	}
	return
}
