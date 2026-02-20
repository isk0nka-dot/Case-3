// Package grpc implements the gRPC transport layer for the Event Collector service.
// It translates between protobuf wire messages and domain entities, following
// the Clean Architecture boundary: transport adapters depend inward on domain.
//
// This package requires generated protobuf Go code. Run the following before building:
//
//	protoc --go_out=. --go-grpc_out=. api/proto/v1/event_collector.proto
package grpc

import (
	"encoding/json"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/domain/valueobject"

	pb "github.com/argus-ai/event-collector/api/proto/v1"
)

// ---------------------------------------------------------------------------
//  Proto -> Domain
// ---------------------------------------------------------------------------

// ProtoEventToDomain converts a protobuf ProctoringEvent to a domain entity.
// The peerAddr parameter is the remote address extracted from the gRPC peer
// context and is used to set the server-side IP address on the client metadata.
//
// The oneof payload field is marshaled back to JSON bytes so the domain entity
// can store it in a serialization-agnostic format (Kafka, ClickHouse, etc.).
func ProtoEventToDomain(pbEvent *pb.ProctoringEvent, peerAddr string) *entity.ProctoringEvent {
	if pbEvent == nil {
		return nil
	}

	e := &entity.ProctoringEvent{
		EventID:        pbEvent.GetEventId(),
		SessionID:      pbEvent.GetSessionId(),
		StudentID:      pbEvent.GetStudentId(),
		ExamID:         pbEvent.GetExamId(),
		OrgID:          pbEvent.GetOrgId(),
		EventType:      protoEventTypeToDomain(pbEvent.GetEventType()),
		Severity:       protoSeverityToDomain(pbEvent.GetSeverity()),
		Source:         protoSourceToDomain(pbEvent.GetSource()),
		ClientTimestamp: protoTimestampToTime(pbEvent.GetClientTimestamp()),
		VideoTimestamp: pbEvent.GetVideoTimestampSec(),
		Label:          pbEvent.GetLabel(),
		Confidence:     pbEvent.GetConfidence(),
	}

	// Marshal the oneof payload to JSON bytes for downstream storage.
	// The PayloadType discriminator is set based on which oneof variant is present.
	e.Payload, e.PayloadType = marshalOneofPayload(pbEvent)

	// Map client metadata. IP address is always set server-side from the
	// gRPC peer address for security — never trust the client-supplied value.
	if meta := pbEvent.GetClientMeta(); meta != nil {
		e.ClientMeta = entity.ClientMeta{
			UserAgent:         meta.GetUserAgent(),
			SDKVersion:        meta.GetSdkVersion(),
			Resolution:        meta.GetResolution(),
			TimezoneOffsetMin: meta.GetTimezoneOffsetMin(),
			IPAddress:         peerAddr,
			Region:            "", // Derived server-side by a separate geo-IP service.
		}
	} else {
		e.ClientMeta = entity.ClientMeta{
			IPAddress: peerAddr,
		}
	}

	return e
}

// ---------------------------------------------------------------------------
//  Domain -> Proto
// ---------------------------------------------------------------------------

// DomainEventToProto converts a domain ProctoringEvent back to its protobuf
// representation. This is primarily used for response messages and for any
// server-to-client streaming scenarios requiring the full event envelope.
func DomainEventToProto(e *entity.ProctoringEvent) *pb.ProctoringEvent {
	if e == nil {
		return nil
	}

	pbEvent := &pb.ProctoringEvent{
		EventId:           e.EventID,
		SessionId:         e.SessionID,
		StudentId:         e.StudentID,
		ExamId:            e.ExamID,
		OrgId:             e.OrgID,
		EventType:         domainEventTypeToProto(e.EventType),
		Severity:          domainSeverityToProto(e.Severity),
		Source:            domainSourceToProto(e.Source),
		ServerTimestamp:   timeToProtoTimestamp(e.ServerTimestamp),
		ClientTimestamp:   timeToProtoTimestamp(e.ClientTimestamp),
		VideoTimestampSec: e.VideoTimestamp,
		Label:             e.Label,
		Confidence:        e.Confidence,
		ClientMeta: &pb.ClientMeta{
			UserAgent:         e.ClientMeta.UserAgent,
			SdkVersion:        e.ClientMeta.SDKVersion,
			Resolution:        e.ClientMeta.Resolution,
			TimezoneOffsetMin: e.ClientMeta.TimezoneOffsetMin,
			IpAddress:         e.ClientMeta.IPAddress,
			Region:            e.ClientMeta.Region,
		},
	}

	// Unmarshal the JSON payload back into the appropriate oneof field.
	unmarshalOneofPayload(pbEvent, e.Payload, e.PayloadType)

	return pbEvent
}

// ---------------------------------------------------------------------------
//  Enum Conversions: Proto -> Domain
// ---------------------------------------------------------------------------

// protoEventTypeToDomain maps the protobuf EventType enum to the domain value object.
// Both enums share identical numeric values by design, so a direct cast is safe.
func protoEventTypeToDomain(pt pb.EventType) valueobject.EventType {
	return valueobject.EventType(pt)
}

// protoSeverityToDomain maps the protobuf Severity enum to the domain value object.
func protoSeverityToDomain(ps pb.Severity) valueobject.Severity {
	return valueobject.Severity(ps)
}

// protoSourceToDomain maps the protobuf EventSource enum to the domain value object.
func protoSourceToDomain(ps pb.EventSource) valueobject.EventSource {
	return valueobject.EventSource(ps)
}

// ---------------------------------------------------------------------------
//  Enum Conversions: Domain -> Proto
// ---------------------------------------------------------------------------

// domainEventTypeToProto maps the domain EventType back to protobuf.
func domainEventTypeToProto(dt valueobject.EventType) pb.EventType {
	return pb.EventType(dt)
}

// domainSeverityToProto maps the domain Severity back to protobuf.
func domainSeverityToProto(ds valueobject.Severity) pb.Severity {
	return pb.Severity(ds)
}

// domainSourceToProto maps the domain EventSource back to protobuf.
func domainSourceToProto(des valueobject.EventSource) pb.EventSource {
	return pb.EventSource(des)
}

// ---------------------------------------------------------------------------
//  Timestamp Helpers
// ---------------------------------------------------------------------------

// protoTimestampToTime converts a protobuf Timestamp to a Go time.Time.
// Returns the zero value if the timestamp is nil.
func protoTimestampToTime(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

// timeToProtoTimestamp converts a Go time.Time to a protobuf Timestamp.
// Returns nil for the zero time value to avoid sending empty timestamps on the wire.
func timeToProtoTimestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// ---------------------------------------------------------------------------
//  Oneof Payload Marshaling
// ---------------------------------------------------------------------------

// marshalOneofPayload extracts the oneof payload variant from the proto message,
// marshals it to JSON bytes, and returns both the bytes and a string discriminator.
//
// JSON is used as the intermediate format because:
//   - It is human-readable for debugging.
//   - ClickHouse natively supports JSON columns.
//   - Kafka consumers in other languages can deserialize without proto definitions.
func marshalOneofPayload(pbEvent *pb.ProctoringEvent) ([]byte, string) {
	var (
		payload proto.Message
		ptype   string
	)

	switch p := pbEvent.GetPayload().(type) {
	case *pb.ProctoringEvent_GazeDeviation:
		payload = p.GazeDeviation
		ptype = "gaze_deviation"
	case *pb.ProctoringEvent_FaceDetection:
		payload = p.FaceDetection
		ptype = "face_detection"
	case *pb.ProctoringEvent_ObjectDetection:
		payload = p.ObjectDetection
		ptype = "object_detection"
	case *pb.ProctoringEvent_Audio:
		payload = p.Audio
		ptype = "audio"
	case *pb.ProctoringEvent_Browser:
		payload = p.Browser
		ptype = "browser"
	case *pb.ProctoringEvent_System:
		payload = p.System
		ptype = "system"
	case *pb.ProctoringEvent_Psychometry:
		payload = p.Psychometry
		ptype = "psychometry"
	case *pb.ProctoringEvent_Network:
		payload = p.Network
		ptype = "network"
	case *pb.ProctoringEvent_Kernel:
		payload = p.Kernel
		ptype = "kernel"
	default:
		return nil, ""
	}

	if payload == nil {
		return nil, ""
	}

	// Use protojson-compatible JSON encoding. We fall back to encoding/json
	// which works for proto-generated structs with json tags. For full proto3
	// JSON compliance, consider google.golang.org/protobuf/encoding/protojson.
	data, err := json.Marshal(payload)
	if err != nil {
		// This should never happen with well-formed proto messages.
		// Return nil rather than propagating errors in a mapper.
		return nil, ""
	}

	return data, ptype
}

// unmarshalOneofPayload deserializes JSON bytes back into the appropriate
// protobuf oneof variant based on the payload type discriminator.
func unmarshalOneofPayload(pbEvent *pb.ProctoringEvent, data []byte, payloadType string) {
	if len(data) == 0 || payloadType == "" {
		return
	}

	switch payloadType {
	case "gaze_deviation":
		var p pb.GazeDeviationPayload
		if json.Unmarshal(data, &p) == nil {
			pbEvent.Payload = &pb.ProctoringEvent_GazeDeviation{GazeDeviation: &p}
		}
	case "face_detection":
		var p pb.FaceDetectionPayload
		if json.Unmarshal(data, &p) == nil {
			pbEvent.Payload = &pb.ProctoringEvent_FaceDetection{FaceDetection: &p}
		}
	case "object_detection":
		var p pb.ObjectDetectionPayload
		if json.Unmarshal(data, &p) == nil {
			pbEvent.Payload = &pb.ProctoringEvent_ObjectDetection{ObjectDetection: &p}
		}
	case "audio":
		var p pb.AudioPayload
		if json.Unmarshal(data, &p) == nil {
			pbEvent.Payload = &pb.ProctoringEvent_Audio{Audio: &p}
		}
	case "browser":
		var p pb.BrowserPayload
		if json.Unmarshal(data, &p) == nil {
			pbEvent.Payload = &pb.ProctoringEvent_Browser{Browser: &p}
		}
	case "system":
		var p pb.SystemPayload
		if json.Unmarshal(data, &p) == nil {
			pbEvent.Payload = &pb.ProctoringEvent_System{System: &p}
		}
	case "psychometry":
		var p pb.PsychometryPayload
		if json.Unmarshal(data, &p) == nil {
			pbEvent.Payload = &pb.ProctoringEvent_Psychometry{Psychometry: &p}
		}
	case "network":
		var p pb.NetworkPayload
		if json.Unmarshal(data, &p) == nil {
			pbEvent.Payload = &pb.ProctoringEvent_Network{Network: &p}
		}
	case "kernel":
		var p pb.KernelPayload
		if json.Unmarshal(data, &p) == nil {
			pbEvent.Payload = &pb.ProctoringEvent_Kernel{Kernel: &p}
		}
	}
}
