from ultralytics import YOLO
import os

print("Exporting YOLOv8n to ONNX...")

models_dir = os.path.join("argus-backend", "ai-sidecar", "models")
os.makedirs(models_dir, exist_ok=True)
yolo_onnx_path = os.path.join(models_dir, "yolov8n.onnx")

try:
    # This downloads yolov8n.pt automatically, then exports it
    model = YOLO("yolov8n.pt")
    # Export it to the models directory
    model.export(format="onnx")
    
    # It creates yolov8n.onnx in the current directory, we move it
    if os.path.exists("yolov8n.onnx"):
        os.rename("yolov8n.onnx", yolo_onnx_path)
    
    print(f"✅ YOLOv8n ONNX model successfully saved to: {yolo_onnx_path}")
except Exception as e:
    print(f"❌ Error: {e}")

