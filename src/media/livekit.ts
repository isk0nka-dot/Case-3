// =============================================================================
// Argus SDK — Optional LiveKit Publisher
// =============================================================================
//
// Publishes student camera/audio tracks into the Argus LiveKit room. The adapter
// intentionally avoids a hard import so SDK consumers can either pass the
// livekit-client module or load the LiveKit UMD bundle globally.
// =============================================================================

import type {
  LiveKitClientModule,
  LiveKitPublishingConfig,
  LiveKitPublishingState,
  LiveKitRoom,
} from '../types';

export interface StudentMediaTokenResponse {
  livekitToken: string;
  livekitUrl: string;
  room: string;
}

export interface LiveKitPublisherOptions {
  serverUrl: string;
  sessionId: string;
  sessionToken: string;
  config?: LiveKitPublishingConfig;
}

export class LiveKitPublisher {
  private readonly serverUrl: string;
  private readonly sessionId: string;
  private readonly sessionToken: string;
  private readonly config?: LiveKitPublishingConfig;
  private room: LiveKitRoom | null = null;
  private state: LiveKitPublishingState = { connected: false };

  constructor(options: LiveKitPublisherOptions) {
    this.serverUrl = options.serverUrl.replace(/\/+$/, '');
    this.sessionId = options.sessionId;
    this.sessionToken = options.sessionToken;
    this.config = options.config;
  }

  get currentState(): LiveKitPublishingState {
    return { ...this.state };
  }

  async start(stream: MediaStream): Promise<LiveKitPublishingState> {
    const client = this._resolveClient();
    const tokenResponse = await this._requestStudentToken();

    const room = new client.Room({
      adaptiveStream: true,
      dynacast: true,
    });

    await room.connect(tokenResponse.livekitUrl, tokenResponse.livekitToken);

    const videoTrack = stream.getVideoTracks()[0];
    const audioTrack = stream.getAudioTracks()[0];

    let videoTrackId = '';
    let audioTrackId = '';

    if (videoTrack) {
      const publication = await room.localParticipant.publishTrack(videoTrack, {
        name: 'argus-camera',
        source: client.Track?.Source?.Camera ?? 'camera',
      });
      videoTrackId = publication.trackSid ?? publication.sid ?? '';
    }

    if (audioTrack) {
      const publication = await room.localParticipant.publishTrack(audioTrack, {
        name: 'argus-microphone',
        source: client.Track?.Source?.Microphone ?? 'microphone',
      });
      audioTrackId = publication.trackSid ?? publication.sid ?? '';
    }

    this.room = room;
    this.state = {
      connected: true,
      room: tokenResponse.room,
      livekitUrl: tokenResponse.livekitUrl,
      videoTrackId,
      audioTrackId,
    };

    const readyStatus = await this._sendRecordingReady(videoTrackId, audioTrackId);
    this.state = {
      ...this.state,
      recordingReadyStatus: readyStatus,
    };

    return this.currentState;
  }

  stop(): void {
    this.room?.disconnect();
    this.room = null;
    this.state = { ...this.state, connected: false };
  }

  destroy(): void {
    this.stop();
  }

  private _resolveClient(): LiveKitClientModule {
    if (this.config?.client) return this.config.client;

    const maybeWindow = globalThis as unknown as {
      LivekitClient?: LiveKitClientModule;
      LiveKitClient?: LiveKitClientModule;
    };

    const client = maybeWindow.LivekitClient ?? maybeWindow.LiveKitClient;
    if (!client?.Room) {
      throw new Error(
        'LiveKit client is not available. Pass liveKit.client or load livekit-client UMD before starting the SDK.',
      );
    }
    return client;
  }

  private async _requestStudentToken(): Promise<StudentMediaTokenResponse> {
    const endpoint = this.config?.tokenEndpoint
      ?? `${this.serverUrl}/api/v1/external/sessions/${encodeURIComponent(this.sessionId)}/student-token`;

    const response = await fetch(endpoint, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${this.sessionToken}`,
        'Content-Type': 'application/json',
      },
    });

    if (!response.ok) {
      throw new Error(`LiveKit student-token failed: HTTP ${response.status}`);
    }

    const body = await response.json() as Partial<StudentMediaTokenResponse>;
    if (!body.livekitToken || !body.livekitUrl || !body.room) {
      throw new Error('LiveKit student-token response is missing livekitToken, livekitUrl, or room');
    }

    return {
      livekitToken: body.livekitToken,
      livekitUrl: body.livekitUrl,
      room: body.room,
    };
  }

  private async _sendRecordingReady(videoTrackId: string, audioTrackId: string): Promise<string> {
    const endpoint = this.config?.recordingReadyEndpoint
      ?? `${this.serverUrl}/api/v1/external/sessions/${encodeURIComponent(this.sessionId)}/recording-ready`;

    const response = await fetch(endpoint, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${this.sessionToken}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        videoTrackId,
        audioTrackId,
      }),
    });

    if (!response.ok) {
      throw new Error(`LiveKit recording-ready failed: HTTP ${response.status}`);
    }

    const body = await response.json().catch(() => ({})) as { status?: string };
    return body.status ?? 'ok';
  }
}
