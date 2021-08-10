package templating

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"

	m "github.com/achelovekov/n9k-modeling/modeling"
)

type ServiceVariablesDB struct {
	Name              string                  `json:"Name"`
	Variables         []ServiceVariablesEntry `json:"Variables"`
	IndirectVariables []string                `json:"IndirectVariables"`
}

type ServiceVariablesEntry struct {
	Key  string         `json:"Key"`
	Data []VariableData `json:"Data"`
}

type VariableData struct {
	Name  string      `json:"Name"`
	Value interface{} `json:"Value"`
}

type ServiceVariablesDBProcessed map[string]map[string]interface{}

func LoadServiceVariablesDBProcessed(sOTDB m.SOTDB) ServiceVariablesDBProcessed {
	serviceVariablesDBProcessed := make(ServiceVariablesDBProcessed)

	for _, dBEntry := range sOTDB.DB {

		data := make(map[string]interface{})

		for _, globalVariablesEntry := range dBEntry.KeyData.GlobalVariables {
			data[globalVariablesEntry.Name] = globalVariablesEntry.Value
		}
		serviceVariablesDBProcessed[dBEntry.KeyID] = data
	}

	return serviceVariablesDBProcessed
}

type IndirectVariablesDB map[string]IndirectVariablesDBData
type IndirectVariablesDBData map[string]IndirectVariablesDBDataForKey
type IndirectVariablesDBDataForKey map[string]interface{}

func LoadIndirectVariablesDB(processedData m.ProcessedData, variablesList []string) IndirectVariablesDB {
	indirectVariablesDB := make(IndirectVariablesDB)

	for _, deviceFootprintDBEntry := range processedData.DeviceFootprintDB {
		indirectVariablesDBData := make(IndirectVariablesDBData)
		indirectVariablesDB[deviceFootprintDBEntry.DeviceName] = indirectVariablesDBData

		for _, deviceDataEntry := range deviceFootprintDBEntry.DeviceData {
			indirectVariablesDBDataForKey := make(IndirectVariablesDBDataForKey)
			indirectVariablesDBData[deviceDataEntry.Key] = indirectVariablesDBDataForKey
			for _, variable := range variablesList {
				if v, ok := deviceDataEntry.Data[variable]; ok {
					indirectVariablesDBDataForKey[variable] = v
				}
			}
		}

	}
	return indirectVariablesDB
}

func TemplateConstruct(serviceName string, serviceFootprintDB m.ServiceFootprintDB, serviceVariablesDBProcessed ServiceVariablesDBProcessed, indirectVariablesDB IndirectVariablesDB, generalTemplateConstructor GeneralTemplateConstructor) m.DeviceFootprintDB {

	var deviceFootprintDB m.DeviceFootprintDB

	for _, serviceFootprintDBEntry := range serviceFootprintDB {

		var deviceFootprintDBEntry m.DeviceFootprintDBEntry
		deviceFootprintDBEntry.DeviceName = serviceFootprintDBEntry.DeviceName
		for _, serviceLayoutEntry := range serviceFootprintDBEntry.ServiceLayouts {

			var deviceDataEntry m.DeviceDataEntry
			deviceDataEntry.Key = serviceLayoutEntry.Key
			m := make(map[string]interface{})
			deviceDataEntry.Data = m
			for _, dataEntry := range serviceLayoutEntry.Data {
				if dataEntry.Value {
					generalTemplateConstructor[serviceName][dataEntry.Name](m, serviceLayoutEntry.Key, serviceVariablesDBProcessed)
				}
			}

			deviceFootprintDBEntry.DeviceData = append(deviceFootprintDBEntry.DeviceData, deviceDataEntry)

		}

		deviceFootprintDB = append(deviceFootprintDB, deviceFootprintDBEntry)
	}

	return deviceFootprintDB
}

type fn func(map[string]interface{}, string, ServiceVariablesDBProcessed)
type GeneralTemplateConstructor map[string]ServiceConstructor
type ServiceConstructor map[string]fn

func LoadGeneralTemplateConstructor() GeneralTemplateConstructor {
	GeneralTemplateConstructor := make(GeneralTemplateConstructor)

	VNIServiceConstructor := make(ServiceConstructor)
	VNIServiceConstructor["L2VNI"] = VNIMakeL2VNITemplate
	VNIServiceConstructor["L3VNI-AC"] = VNIMakeL3VNIACTemplate
	VNIServiceConstructor["L3VNI-AG"] = VNIMakeL3VNIAGTemplate
	VNIServiceConstructor["AGW"] = VNIMakeAGWTemplate
	VNIServiceConstructor["PIM"] = VNIMakePIMTemplate
	VNIServiceConstructor["IR"] = VNIMakeIRTemplate
	VNIServiceConstructor["MS-IR"] = VNIMakeMSIRTemplate
	VNIServiceConstructor["ARP-Suppress"] = VNIMakeARPSuppressTemplate

	GeneralTemplateConstructor["VNI"] = VNIServiceConstructor

	OSPFServiceConstructor := make(ServiceConstructor)
	OSPFServiceConstructor["Interface"] = OSPFMakeInterfaceTemplate
	OSPFServiceConstructor["LSA-control"] = OSPFMakeLSAControlTemplate
	OSPFServiceConstructor["BFD"] = OSPFMakeBFDTemplate
	OSPFServiceConstructor["Isolate"] = OSPFMakeIsolateTemplate

	GeneralTemplateConstructor["OSPF"] = OSPFServiceConstructor

	BGPServiceConstructor := make(ServiceConstructor)
	BGPServiceConstructor["L2VPN-EVPN"] = BGPMakeL2VPNEVPNTemplate
	BGPServiceConstructor["IPv4"] = BGPMakeIPv4Template
	BGPServiceConstructor["BFD"] = BGPMakeBFDTemplate
	BGPServiceConstructor["Template"] = BGPMakeTemplateTemplate
	BGPServiceConstructor["Isolate"] = BGPMakeIsolateTemplate

	GeneralTemplateConstructor["BGP"] = BGPServiceConstructor

	return GeneralTemplateConstructor
}

func VNIMakeL2VNITemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["L2VNI.l2BD.accEncap"] = "vxlan-" + serviceVariablesDB[key]["VNID"].(string)
	M["L2VNI.l2BD.id"] = serviceVariablesDB[key]["VNID"].(string)[3:]
	M["L2VNI.l2BD.name"] = serviceVariablesDB[key]["ZoneName"].(string) + "_" + serviceVariablesDB[key]["Subnet"].(string) + "/" + serviceVariablesDB[key]["Mask"].(string)
	M["L2VNI.rtctrlRttEntry.rtt.export"] = "route-target:as2-nn4:" + serviceVariablesDB[key]["evpnAS"].(string) + ":" + serviceVariablesDB[key]["VNID"].(string)
	M["L2VNI.rtctrlRttEntry.rtt.import"] = "route-target:as2-nn4:" + serviceVariablesDB[key]["evpnAS"].(string) + ":" + serviceVariablesDB[key]["VNID"].(string)
	M["L2VNI.nvoNw.suppressARP"] = "off"
}

func VNIMakeL3VNIACTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["L3VNI.bgpDomAf.maxEcmp"] = serviceVariablesDB[key]["maxEcmp"].(string)
	M["L3VNI.bgpGr.staleIntvl"] = "1800"
	M["L3VNI.bgpInst.asn"] = serviceVariablesDB[key]["bgpAsn"].(string)
	M["L3VNI.bgpInterLeakP.rtMap.direct"] = serviceVariablesDB[key]["rtMap.direct"].(string)
	M["L3VNI.bgpPathCtrl.asPathMultipathRelax"] = "enabled"
	M["L3VNI.ipv4If.forward"] = "enabled"
	M["L3VNI.l2BD.id"] = serviceVariablesDB[key]["L3VNI.l2BD.id"].(string)
	M["L3VNI.l3Inst.encap"] = "vxlan-" + serviceVariablesDB[key]["L3VNI.l3Inst.encap"].(string)
	M["L3VNI.rtctrlRttEntry.rtt.import.ipv4-ucast"] = "route-target:as2-nn4:" + serviceVariablesDB[key]["evpnAS"].(string) + ":" + serviceVariablesDB[key]["L3VNI.l3Inst.encap"].(string)
	M["L3VNI.rtctrlRttEntry.rtt.export.l2vpn-evpn"] = "route-target:as2-nn4:" + serviceVariablesDB[key]["evpnAS"].(string) + ":" + serviceVariablesDB[key]["L3VNI.l3Inst.encap"].(string)
	M["L3VNI.rtctrlRttEntry.rtt.import.ipv4-ucast"] = "route-target:as2-nn4:" + serviceVariablesDB[key]["evpnAS"].(string) + ":" + serviceVariablesDB[key]["L3VNI.l3Inst.encap"].(string)
	M["L3VNI.rtctrlRttEntry.rtt.export.l2vpn-evpn"] = "route-target:as2-nn4:" + serviceVariablesDB[key]["evpnAS"].(string) + ":" + serviceVariablesDB[key]["L3VNI.l3Inst.encap"].(string)
	M["L3VNI.rtmapMatchRtTag.tag"] = serviceVariablesDB[key]["L3VNI.l2BD.id"].(string)
	M["L3VNI.sviIf.id"] = "vlan" + serviceVariablesDB[key]["L3VNI.l2BD.id"].(string)
	M["L3VNI.nvoNw.associateVrfFlag"] = "yes"
}

func VNIMakeL3VNIAGTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["L3VNI.bgpDomAf.maxEcmp"] = serviceVariablesDB[key]["maxEcmp"].(string)
	M["L3VNI.bgpGr.staleIntvl"] = "1800"
	M["L3VNI.bgpInst.asn"] = serviceVariablesDB[key]["bgpAsn"].(string)
	M["L3VNI.bgpInterLeakP.rtMap.direct"] = serviceVariablesDB[key]["rtMap.direct"].(string)
	M["L3VNI.bgpPathCtrl.asPathMultipathRelax"] = "enabled"
	M["L3VNI.ipv4If.forward"] = "enabled"
	M["L3VNI.l2BD.id"] = serviceVariablesDB[key]["L3VNI.l2BD.id"].(string)
	M["L3VNI.l3Inst.encap"] = "vxlan-" + serviceVariablesDB[key]["L3VNI.l3Inst.encap"].(string)
	M["L3VNI.rtctrlRttEntry.rtt.import.ipv4-ucast"] = "route-target:as2-nn4:" + serviceVariablesDB[key]["evpnAS"].(string) + ":" + serviceVariablesDB[key]["L3VNI.l3Inst.encap"].(string)
	M["L3VNI.rtctrlRttEntry.rtt.export.l2vpn-evpn"] = "route-target:as2-nn4:" + serviceVariablesDB[key]["evpnAS"].(string) + ":" + serviceVariablesDB[key]["L3VNI.l3Inst.encap"].(string)
	M["L3VNI.rtctrlRttEntry.rtt.import.ipv4-ucast"] = "route-target:as2-nn4:" + serviceVariablesDB[key]["evpnAS"].(string) + ":" + serviceVariablesDB[key]["L3VNI.l3Inst.encap"].(string)
	M["L3VNI.rtctrlRttEntry.rtt.export.l2vpn-evpn"] = "route-target:as2-nn4:" + serviceVariablesDB[key]["evpnAS"].(string) + ":" + serviceVariablesDB[key]["L3VNI.l3Inst.encap"].(string)
	M["L3VNI.rtmapMatchRtTag.tag"] = serviceVariablesDB[key]["L3VNI.l2BD.id"].(string)
	M["L3VNI.sviIf.id"] = "vlan" + serviceVariablesDB[key]["L3VNI.l2BD.id"].(string)
	M["L3VNI.nvoNw.associateVrfFlag"] = "yes"
}

func VNIMakeAGWTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["L2VNI.hmmFwdIf.mode"] = "anycastGW"
	M["L2VNI.ipv4Addr.addr"] = serviceVariablesDB[key]["IPAddress"].(string) + "/" + serviceVariablesDB[key]["Mask"].(string)
	M["L2VNI.ipv4Addr.tag"] = "39" + serviceVariablesDB[key]["ZoneID"].(string)
	M["L2VNI.ipv4Dom.name"] = serviceVariablesDB[key]["ZoneName"].(string)
	M["L2VNI.sviIf.id"] = "vlan" + serviceVariablesDB[key]["VNID"].(string)[3:]
}

func VNIMakeIRTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["L2VNI.nvoNw.mcastGroup"] = "0.0.0.0"
	M["L2VNI.nvoNw.multisiteIngRepl"] = "disable"
	M["L2VNI.nvoNw.vni"] = serviceVariablesDB[key]["VNID"].(string)
	M["L2VNI.nvoIngRepl.proto"] = "bgp"
	M["L2VNI.nvoIngRepl.rn"] = "IngRepl"
}

func VNIMakePIMTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["L2VNI.nvoNw.mcastGroup"] = "225.1.0." + serviceVariablesDB[key]["ZoneIDForMcast"].(string)
	M["L2VNI.nvoNw.multisiteIngRepl"] = "disable"
	M["L2VNI.nvoNw.vni"] = serviceVariablesDB[key]["VNID"].(string)
}

func VNIMakeMSIRTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["L2VNI.nvoNw.multisiteIngRepl"] = "enable"
}

func VNIMakeARPSuppressTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["L2VNI.nvoNw.suppressARP"] = "enabled"
}

func OSPFMakeInterfaceTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["ospfIf.adminSt"] = "enabled"
	M["ospfIf.area"] = serviceVariablesDB[key]["Area"].(string)
	M["ospfIf.helloIntvl"], _ = strconv.ParseInt(serviceVariablesDB[key]["HelloIntvl"].(string), 10, 64)
	M["ospfIf.nwT"] = serviceVariablesDB[key]["nwT"].(string)
	M["ospfIf.rexmitIntvl"], _ = strconv.ParseInt(serviceVariablesDB[key]["RexmitIntvl"].(string), 10, 64)
	M["ospfInst.ctrl"] = ""
}

func OSPFMakeLSAControlTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["ospfLsaCtrl.arrivalIntvl"], _ = strconv.ParseInt(serviceVariablesDB[key]["ArrivalIntvl"].(string), 10, 64)
	M["ospfLsaCtrl.gpPacingIntvl"], _ = strconv.ParseInt(serviceVariablesDB[key]["GpPacingIntvl"].(string), 10, 64)
	M["ospfLsaCtrl.holdIntvl"], _ = strconv.ParseInt(serviceVariablesDB[key]["HoldIntvl"].(string), 10, 64)
	M["ospfLsaCtrl.maxIntvl"], _ = strconv.ParseInt(serviceVariablesDB[key]["MaxIntvl"].(string), 10, 64)
}

func OSPFMakeBFDTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["ospfIf.bfdCtrl"] = "enabled"
	M["ospfDom.ctrl"] = "bfd"
}

func OSPFMakeIsolateTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["ospfInst.ctrl"] = "isolate"
}

func BGPMakeL2VPNEVPNTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["bgpDom.name"] = serviceVariablesDB[key]["bgpDom.name"].(string)
	M["bgpDomAf.advPip.l2vpn-evpn"] = serviceVariablesDB[key]["advPip.l2vpn-evpn"].(string)

	res, err := strconv.ParseInt(serviceVariablesDB[key]["critNhTimeout.l2vpn-evpn"].(string), 10, 64)
	if err != nil {
		M["bgpDomAf.critNhTimeout.l2vpn-evpn"] = serviceVariablesDB[key]["critNhTimeout.l2vpn-evpn"].(string)
	} else {
		M["bgpDomAf.critNhTimeout.l2vpn-evpn"] = res
	}

	res, err = strconv.ParseInt(serviceVariablesDB[key]["nonCritNhTimeout.l2vpn-evpn"].(string), 10, 64)
	if err != nil {
		M["bgpDomAf.nonCritNhTimeout.l2vpn-evpn"] = serviceVariablesDB[key]["nonCritNhTimeout.l2vpn-evpn"].(string)
	} else {
		M["bgpDomAf.nonCritNhTimeout.l2vpn-evpn"] = res
	}

	M["bgpMaxPfxP.maxPfx.l2vpn-evpn"], _ = strconv.ParseInt(serviceVariablesDB[key]["maxPfx.l2vpn-evpn"].(string), 10, 64)
	M["bgpPeerAf.sendComExt.l2vpn-evpn"] = serviceVariablesDB[key]["sendComExt.l2vpn-evpn"].(string)
	M["bgpPeerAf.sendComStd.l2vpn-evpn"] = serviceVariablesDB[key]["sendComStd.l2vpn-evpn"].(string)
	M["bgpInst.isolate"] = "disabled"
}

func BGPMakeIPv4Template(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["bgpDom.name"] = serviceVariablesDB[key]["bgpDom.name"].(string)
	M["bgpDomAf.advPip.ipv4-ucast"] = serviceVariablesDB[key]["advPip.ipv4-ucast"].(string)

	res, err := strconv.ParseInt(serviceVariablesDB[key]["critNhTimeout.ipv4-ucast"].(string), 10, 64)
	if err != nil {
		M["bgpDomAf.critNhTimeout.ipv4-ucast"] = serviceVariablesDB[key]["critNhTimeout.ipv4-ucast"].(string)
	} else {
		M["bgpDomAf.critNhTimeout.ipv4-ucast"] = res
	}

	res, err = strconv.ParseInt(serviceVariablesDB[key]["nonCritNhTimeout.ipv4-ucast"].(string), 10, 64)
	if err != nil {
		M["bgpDomAf.nonCritNhTimeout.ipv4-ucast"] = serviceVariablesDB[key]["nonCritNhTimeout.ipv4-ucast"].(string)
	} else {
		M["bgpDomAf.nonCritNhTimeout.ipv4-ucast"] = res
	}

	M["bgpPeerAf.sendComExt.ipv4-ucast"] = serviceVariablesDB[key]["sendComExt.ipv4-ucast"].(string)
	M["bgpPeerAf.sendComStd.ipv4-ucast"] = serviceVariablesDB[key]["sendComStd.ipv4-ucast"].(string)
	M["bgpInst.isolate"] = "disabled"
}

func BGPMakeBFDTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["bgpPeerCont.ctrl"] = serviceVariablesDB[key]["bgpPeerCont.ctrl"].(string)
}

func BGPMakeTemplateTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["bgpPeer.peerImp"] = serviceVariablesDB[key]["bgpPeer.peerImp"].(string)
}

func BGPMakeIsolateTemplate(M map[string]interface{}, key string, serviceVariablesDB ServiceVariablesDBProcessed) {
	M["bgpInst.isolate"] = "enabled"
}

func PrettyPrint(src interface{}) {
	JSONData, err := json.MarshalIndent(&src, "", "  ")
	if err != nil {
		log.Fatalf(err.Error())
	}
	fmt.Printf("Pretty processed output: %s\n", string(JSONData))
}

func LoadProcessedData(fineName string) m.ProcessedData {
	var ProcessedData m.ProcessedData
	ProcessedDataFile, err := os.Open(fineName)
	if err != nil {
		fmt.Println(err)
	}
	defer ProcessedDataFile.Close()

	ProcessedDataFileBytes, _ := ioutil.ReadAll(ProcessedDataFile)

	err = json.Unmarshal(ProcessedDataFileBytes, &ProcessedData)
	if err != nil {
		fmt.Println(err)
	}

	return ProcessedData
}

func FindCommonKeys(src map[string]interface{}, dst map[string]interface{}) []string {
	result := make([]string, 0)
	for k, _ := range src {
		if _, ok := dst[k]; ok {
			result = append(result, k)
		}
	}
	return result
}

func FindDistinctKeys(src map[string]interface{}, dst map[string]interface{}) []string {
	result := make([]string, 0)
	for k, _ := range src {
		if _, ok := dst[k]; !ok {
			result = append(result, k)
		}
	}
	return result
}

type DeviceDiffDB []DeviceDiffDBEntry
type DeviceDiffDBEntry struct {
	DeviceName string          `bson:"DeviceName"`
	DiffData   []DiffDataEntry `bson:"DiffData"`
}
type DiffDataEntry struct {
	Key      string                 `bson:"Key"`
	ToChange map[string]interface{} `bson:"ToChange"`
	ToAdd    map[string]interface{} `bson:"ToAdd"`
	//ToDelete map[string]interface{} `bson:"ToDelete"`
}

func ConstrustDiffDataEntry(t map[string]interface{}, o map[string]interface{}, key string) DiffDataEntry {

	var diffDataEntry DiffDataEntry

	ToChange := make(map[string]interface{})
	ToAdd := make(map[string]interface{})
	//ToDelete := make(map[string]interface{})

	diffDataEntry.Key = key
	diffDataEntry.ToChange = ToChange
	diffDataEntry.ToAdd = ToAdd
	//diffDataEntry.ToDelete = ToDelete

	commonKeys := FindCommonKeys(t, o)
	distinctKeysSrcOnly := FindDistinctKeys(t, o)
	//distinctKeysDstOnly := FindDistinctKeys(t, o)

	for _, v := range commonKeys {
		if t[v] != o[v] {
			ToChange[v] = t[v]
		}
	}

	for _, v := range distinctKeysSrcOnly {
		ToAdd[v] = t[v]
	}

	/* 	for _, v := range distinctKeysDstOnly {
		ToDelete[v] = o[v]
	} */

	/* 	fmt.Println("toChange", diffDataEntry.ToChange)
	   	fmt.Println("toAdd", diffDataEntry.ToAdd) */

	return diffDataEntry

}

func CheckForChanges(deviceDiffDB DeviceDiffDB) []string {

	var result []string

	for _, deviceDiffDBEntry := range deviceDiffDB {

		var marker bool

		for _, diffDataEntry := range deviceDiffDBEntry.DiffData {
			//&& len(diffDataEntry.ToDelete)
			if len(diffDataEntry.ToAdd) == 0 && len(diffDataEntry.ToChange) == 0 {
				continue
			} else {
				marker = marker || true
			}
		}

		if marker {
			result = append(result, deviceDiffDBEntry.DeviceName)
		}
	}
	return result
}

func ServiceComponentBitMapConstruct(serviceComponentBitMaps m.ServiceComponentBitMaps) map[string]bool {
	result := make(map[string]bool)
	for _, serviceComponentBitMap := range serviceComponentBitMaps {
		result[serviceComponentBitMap.Name] = serviceComponentBitMap.Value
	}
	return result
}

type SOTTemplatingReference []SOTTemplatingReferenceEntry
type SOTTemplatingReferenceEntry struct {
	KeyID   string
	KeyData []KeyDataEntry
}

type KeyDataEntry struct {
	DeviceType   string
	VariablesMap map[string]interface{}
}

func GetSOTTemplatingReference(sOTDB m.SOTDB, localServiceDefinitions m.LocalServiceDefinitions, serviceVariablesDB ServiceVariablesDBProcessed, generalTemplateConstructor GeneralTemplateConstructor, serviceName string) SOTTemplatingReference {

	var sOTTemplatingReference SOTTemplatingReference

	localServiceMap := m.GetLocalServiceMapFromDefinitions(localServiceDefinitions)

	for _, dBEntry := range sOTDB.DB {
		var sOTTemplatingReferenceEntry SOTTemplatingReferenceEntry
		sOTTemplatingReferenceEntry.KeyID = dBEntry.KeyID

		for _, typeEntry := range dBEntry.KeyData.Types {
			var keyDataEntry KeyDataEntry
			keyDataEntry.DeviceType = typeEntry.DeviceType

			m := make(map[string]interface{})
			for componentName, componentValue := range localServiceMap[typeEntry.LocalServiceType] {
				if componentValue {
					generalTemplateConstructor[serviceName][componentName](m, dBEntry.KeyID, serviceVariablesDB)
				}
			}
			keyDataEntry.VariablesMap = m
			sOTTemplatingReferenceEntry.KeyData = append(sOTTemplatingReferenceEntry.KeyData, keyDataEntry)
		}

		sOTTemplatingReference = append(sOTTemplatingReference, sOTTemplatingReferenceEntry)
	}

	return sOTTemplatingReference
}

func ComplienceReport(processedData m.ProcessedData, sOTTemplatingReference SOTTemplatingReference) DeviceDiffDB {

	var deviceDiffDB DeviceDiffDB

	for _, serviceFootprintDBEntry := range processedData.ServiceTypeDB {

		var deviceDiffDBEntry DeviceDiffDBEntry

		deviceDiffDBEntry.DeviceName = serviceFootprintDBEntry.DeviceName

		for _, typeEntry := range serviceFootprintDBEntry.ServiceTypes {
			if typeEntry.Type != "not-exist" {
				originalData := GetOriginalData(processedData, serviceFootprintDBEntry.DeviceName, typeEntry.Key)
				templatedData := GetTemplatedData(sOTTemplatingReference, serviceFootprintDBEntry.DeviceType, typeEntry.Key)
				diffDataEntry := ConstrustDiffDataEntry(templatedData, originalData, typeEntry.Key)
				if (len(diffDataEntry.ToAdd) > 0) || (len(diffDataEntry.ToChange) > 0) {
					deviceDiffDBEntry.DiffData = append(deviceDiffDBEntry.DiffData, diffDataEntry)
				}
			}
		}
		if len(deviceDiffDBEntry.DiffData) > 0 {
			deviceDiffDB = append(deviceDiffDB, deviceDiffDBEntry)
		}
	}
	return deviceDiffDB
}

func GetOriginalData(processedData m.ProcessedData, deviceName string, key string) map[string]interface{} {
	for _, deviceFootprintDBEntry := range processedData.DeviceFootprintDB {
		if deviceFootprintDBEntry.DeviceName == deviceName {
			for _, deviceDataEntry := range deviceFootprintDBEntry.DeviceData {
				if deviceDataEntry.Key == key {
					return deviceDataEntry.Data
				}
			}
		}
	}
	return map[string]interface{}{}
}

func GetTemplatedData(sOTTemplatingReference SOTTemplatingReference, deviceType string, key string) map[string]interface{} {
	for _, sOTTemplatingReferenceEntry := range sOTTemplatingReference {
		if sOTTemplatingReferenceEntry.KeyID == key {
			for _, keyDataEntry := range sOTTemplatingReferenceEntry.KeyData {
				if keyDataEntry.DeviceType == deviceType {
					return keyDataEntry.VariablesMap
				}
			}
		}
	}

	return map[string]interface{}{}
}
