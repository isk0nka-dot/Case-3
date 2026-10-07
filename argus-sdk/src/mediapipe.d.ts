// Type declarations for @mediapipe/tasks-vision
// The package doesn't ship its own .d.ts files.

declare module '@mediapipe/tasks-vision' {
  export class FilesetResolver {
    static forVisionTasks(wasmPath: string): Promise<FilesetResolver>;
  }

  export interface FaceLandmarkerOptions {
    baseOptions: {
      modelAssetPath: string;
      delegate?: 'GPU' | 'CPU';
    };
    runningMode: 'IMAGE' | 'VIDEO';
    numFaces?: number;
    minFaceDetectionConfidence?: number;
    minFacePresenceConfidence?: number;
    minTrackingConfidence?: number;
    outputFaceBlendshapes?: boolean;
    outputFacialTransformationMatrixes?: boolean;
  }

  export interface FaceLandmarkerResult {
    faceLandmarks: Array<Array<{ x: number; y: number; z: number }>>;
    faceBlendshapes?: Array<{
      categories: Array<{ categoryName: string; score: number }>;
    }>;
  }

  export class FaceLandmarker {
    static createFromOptions(
      resolver: FilesetResolver,
      options: FaceLandmarkerOptions,
    ): Promise<FaceLandmarker>;

    detect(image: HTMLVideoElement | HTMLImageElement): FaceLandmarkerResult;
    detectForVideo(video: HTMLVideoElement, timestamp: number): FaceLandmarkerResult;
    close(): void;
  }
}
