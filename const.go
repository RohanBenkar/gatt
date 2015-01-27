package gatt

// This file includes constants from the BLE spec.

var (
	AttrGAPUUID  = UUID16(0x1800)
	AttrGATTUUID = UUID16(0x1801)

	AttrPrimaryServiceUUID   = UUID16(0x2800)
	AttrSecondaryServiceUUID = UUID16(0x2801)
	AttrIncludeUUID          = UUID16(0x2802)
	AttrCharacteristicUUID   = UUID16(0x2803)

	AttrClientCharacteristicConfigUUID = UUID16(0x2902)
	AttrServerCharacteristicConfigUUID = UUID16(0x2903)

	AttrDeviceNameUUID        = UUID16(0x2A00)
	AttrAppearanceUUID        = UUID16(0x2A01)
	AttrPeripheralPrivacyUUID = UUID16(0x2A02)
	AttrReconnectionAddrUUID  = UUID16(0x2A03)
	AttrPeferredParamsUUID    = UUID16(0x2A04)
	AttrServiceChangedUUID    = UUID16(0x2A05)
)

// https://developer.bluetooth.org/gatt/characteristics/Pages/CharacteristicViewer.aspx?u=org.bluetooth.characteristic.gap.appearance.xml
var gapCharAppearanceGenericComputer = []byte{0x00, 0x80}

const (
	gattCCCNotifyFlag   = 0x0001
	gattCCCIndicateFlag = 0x0002
)

const (
	attOpError              = 0x01
	attOpMtuReq             = 0x02
	attOpMtuRsp             = 0x03
	attOpFindInfoReq        = 0x04
	attOpFindInfoRsp        = 0x05
	attOpFindByTypeValueReq = 0x06
	attOpFindByTypeValueRsp = 0x07
	attOpReadByTypeReq      = 0x08
	attOpReadByTypeRsp      = 0x09
	attOpReadReq            = 0x0a
	attOpReadRsp            = 0x0b
	attOpReadBlobReq        = 0x0c
	attOpReadBlobRsp        = 0x0d
	attOpReadMultiReq       = 0x0e
	attOpReadMultiRsp       = 0x0f
	attOpReadByGroupReq     = 0x10
	attOpReadByGroupRsp     = 0x11
	attOpWriteReq           = 0x12
	attOpWriteRsp           = 0x13
	attOpWriteCmd           = 0x52
	attOpPrepWriteReq       = 0x16
	attOpPrepWriteRsp       = 0x17
	attOpExecWriteReq       = 0x18
	attOpExecWriteRsp       = 0x19
	attOpHandleNotify       = 0x1b
	attOpHandleInd          = 0x1d
	attOpHandleCnf          = 0x1e
	attOpSignedWriteCmd     = 0xd2
)

const (
	attEcodeSuccess           = 0x00
	attEcodeInvalidHandle     = 0x01
	attEcodeReadNotPerm       = 0x02
	attEcodeWriteNotPerm      = 0x03
	attEcodeInvalidPDU        = 0x04
	attEcodeAuthentication    = 0x05
	attEcodeReqNotSupp        = 0x06
	attEcodeInvalidOffset     = 0x07
	attEcodeAuthorization     = 0x08
	attEcodePrepQueueFull     = 0x09
	attEcodeAttrNotFound      = 0x0a
	attEcodeAttrNotLong       = 0x0b
	attEcodeInsuffEncrKeySize = 0x0c
	attEcodeInvalAttrValueLen = 0x0d
	attEcodeUnlikely          = 0x0e
	attEcodeInsuffEnc         = 0x0f
	attEcodeUnsuppGrpType     = 0x10
	attEcodeInsuffResources   = 0x11
)

func attErrorRsp(op byte, h uint16, s uint8) []byte {
	return attErr{opcode: op, attr: h, status: s}.Marshal()
}

// attRspFor maps from att request
// codes to att response codes.
var attRspFor = map[byte]byte{
	attOpMtuReq:             attOpMtuRsp,
	attOpFindInfoReq:        attOpFindInfoRsp,
	attOpFindByTypeValueReq: attOpFindByTypeValueRsp,
	attOpReadByTypeReq:      attOpReadByTypeRsp,
	attOpReadReq:            attOpReadRsp,
	attOpReadBlobReq:        attOpReadBlobRsp,
	attOpReadMultiReq:       attOpReadMultiRsp,
	attOpReadByGroupReq:     attOpReadByGroupRsp,
	attOpWriteReq:           attOpWriteRsp,
	attOpPrepWriteReq:       attOpPrepWriteRsp,
	attOpExecWriteReq:       attOpExecWriteRsp,
}

type attErr struct {
	opcode uint8
	attr   uint16
	status uint8
}

// TODO: Reformulate in a way that lets the caller avoid allocs.
// Accept a []byte? Write directly to an io.Writer?
func (e attErr) Marshal() []byte {
	// little-endian encoding for attr
	return []byte{attOpError, e.opcode, byte(e.attr), byte(e.attr >> 8), e.status}
}
