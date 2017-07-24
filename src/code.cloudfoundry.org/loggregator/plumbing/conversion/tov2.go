package conversion

import (
	"encoding/binary"
	"fmt"
	"strings"

	v2 "code.cloudfoundry.org/loggregator/plumbing/v2"

	"github.com/cloudfoundry/sonde-go/events"
)

// ToV2 converts v1 envelopes up to v2 envelopes.
func ToV2(e *events.Envelope) *v2.Envelope {
	v2e := &v2.Envelope{
		Timestamp: e.GetTimestamp(),
	}
	initTags(v2e, e.GetTags())

	setV2Tag(v2e, "origin", e.GetOrigin())
	setV2Tag(v2e, "deployment", e.GetDeployment())
	setV2Tag(v2e, "job", e.GetJob())
	setV2Tag(v2e, "index", e.GetIndex())
	setV2Tag(v2e, "ip", e.GetIp())
	setV2Tag(v2e, "__v1_type", e.GetEventType().String())

	unsetV2Tag(v2e, "source_id")
	sourceId, ok := e.GetTags()["source_id"]
	v2e.SourceId = sourceId
	if !ok {
		v2e.SourceId = e.GetDeployment() + "/" + e.GetJob()
	}

	unsetV2Tag(v2e, "instance_id")
	v2e.InstanceId = e.GetTags()["instance_id"]

	switch e.GetEventType() {
	case events.Envelope_LogMessage:
		convertLogMessage(v2e, e)
	case events.Envelope_HttpStartStop:
		convertHTTPStartStop(v2e, e)
	case events.Envelope_ValueMetric:
		convertValueMetric(v2e, e)
	case events.Envelope_CounterEvent:
		convertCounterEvent(v2e, e)
	case events.Envelope_Error:
		convertError(v2e, e)
	case events.Envelope_ContainerMetric:
		convertContainerMetric(v2e, e)
	}

	return v2e
}

func setV2Tag(e *v2.Envelope, key string, value interface{}) {
	e.GetDeprecatedTags()[key] = valueText(fmt.Sprintf("%v", value))
}

func unsetV2Tag(e *v2.Envelope, key string) {
	delete(e.GetDeprecatedTags(), key)
}

func initTags(e *v2.Envelope, oldTags map[string]string) {
	e.DeprecatedTags = make(map[string]*v2.Value)
	for k, v := range oldTags {
		setV2Tag(e, k, v)
	}
}

func convertError(v2e *v2.Envelope, v1e *events.Envelope) {
	t := v1e.GetError()
	setV2Tag(v2e, "source", t.GetSource())
	setV2Tag(v2e, "code", t.GetCode())

	v2e.Message = &v2.Envelope_Log{
		Log: &v2.Log{
			Payload: []byte(t.GetMessage()),
			Type:    v2.Log_OUT,
		},
	}
}

func convertAppUUID(appID *events.UUID, sourceID string) string {
	if appID.GetLow() == 0 && appID.GetHigh() == 0 {
		return sourceID
	}
	return uuidToString(appID)
}

func convertAppID(appID, sourceID string) string {
	if appID == "" {
		return sourceID
	}
	return appID
}

func convertHTTPStartStop(v2e *v2.Envelope, v1e *events.Envelope) {
	t := v1e.GetHttpStartStop()
	v2e.SourceId = convertAppUUID(t.GetApplicationId(), v2e.SourceId)
	v2e.Message = &v2.Envelope_Timer{
		Timer: &v2.Timer{
			Name:  "http",
			Start: t.GetStartTimestamp(),
			Stop:  t.GetStopTimestamp(),
		},
	}
	setV2Tag(v2e, "request_id", uuidToString(t.GetRequestId()))
	setV2Tag(v2e, "peer_type", t.GetPeerType().String())
	setV2Tag(v2e, "method", t.GetMethod().String())
	setV2Tag(v2e, "uri", t.GetUri())
	setV2Tag(v2e, "remote_address", t.GetRemoteAddress())
	setV2Tag(v2e, "user_agent", t.GetUserAgent())
	setV2Tag(v2e, "status_code", t.GetStatusCode())
	setV2Tag(v2e, "content_length", t.GetContentLength())
	setV2Tag(v2e, "instance_index", t.GetInstanceIndex())
	setV2Tag(v2e, "routing_instance_id", t.GetInstanceId())
	setV2Tag(v2e, "forwarded", strings.Join(t.GetForwarded(), "\n"))
}

func convertLogMessageType(t events.LogMessage_MessageType) v2.Log_Type {
	name := events.LogMessage_MessageType_name[int32(t)]
	return v2.Log_Type(v2.Log_Type_value[name])
}

func convertLogMessage(v2e *v2.Envelope, e *events.Envelope) {
	t := e.GetLogMessage()
	setV2Tag(v2e, "source_type", t.GetSourceType())
	v2e.InstanceId = t.GetSourceInstance()
	v2e.SourceId = convertAppID(t.GetAppId(), v2e.SourceId)

	v2e.Message = &v2.Envelope_Log{
		Log: &v2.Log{
			Payload: t.GetMessage(),
			Type:    convertLogMessageType(t.GetMessageType()),
		},
	}
}

func convertValueMetric(v2e *v2.Envelope, e *events.Envelope) {
	t := e.GetValueMetric()
	v2e.Message = &v2.Envelope_Gauge{
		Gauge: &v2.Gauge{
			Metrics: map[string]*v2.GaugeValue{
				t.GetName(): {
					Unit:  t.GetUnit(),
					Value: t.GetValue(),
				},
			},
		},
	}
}

func convertCounterEvent(v2e *v2.Envelope, e *events.Envelope) {
	t := e.GetCounterEvent()
	v2e.Message = &v2.Envelope_Counter{
		Counter: &v2.Counter{
			Name: t.GetName(),
			Value: &v2.Counter_Total{
				Total: t.GetTotal(),
			},
		},
	}
}

func convertContainerMetric(v2e *v2.Envelope, e *events.Envelope) {
	t := e.GetContainerMetric()
	v2e.SourceId = convertAppID(t.GetApplicationId(), v2e.SourceId)
	v2e.Message = &v2.Envelope_Gauge{
		Gauge: &v2.Gauge{
			Metrics: map[string]*v2.GaugeValue{
				"instance_index": {
					Unit:  "index",
					Value: float64(t.GetInstanceIndex()),
				},
				"cpu": {
					Unit:  "percentage",
					Value: t.GetCpuPercentage(),
				},
				"memory": {
					Unit:  "bytes",
					Value: float64(t.GetMemoryBytes()),
				},
				"disk": {
					Unit:  "bytes",
					Value: float64(t.GetDiskBytes()),
				},
				"memory_quota": {
					Unit:  "bytes",
					Value: float64(t.GetMemoryBytesQuota()),
				},
				"disk_quota": {
					Unit:  "bytes",
					Value: float64(t.GetDiskBytesQuota()),
				},
			},
		},
	}
}

func valueText(s string) *v2.Value {
	return &v2.Value{&v2.Value_Text{Text: s}}
}

func valueInt64(i int64) *v2.Value {
	return &v2.Value{&v2.Value_Integer{Integer: i}}
}

func valueInt32(i int32) *v2.Value {
	return &v2.Value{&v2.Value_Integer{Integer: int64(i)}}
}

func valueTextSlice(s []string) *v2.Value {
	text := strings.Join(s, "\n")
	return &v2.Value{&v2.Value_Text{Text: text}}
}

func uuidToString(uuid *events.UUID) string {
	low := make([]byte, 8)
	high := make([]byte, 8)
	binary.LittleEndian.PutUint64(low, uuid.GetLow())
	binary.LittleEndian.PutUint64(high, uuid.GetHigh())
	return fmt.Sprintf("%x-%x-%x-%x-%x", low[:4], low[4:6], low[6:], high[:2], high[2:])
}
