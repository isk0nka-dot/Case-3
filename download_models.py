import urllib.request
import os
import sys

print("Скачивание моделей ИИ (YOLOv8 и ArcFace)...")

models_dir = os.path.join("argus-backend", "ai-sidecar", "models")
os.makedirs(models_dir, exist_ok=True)

# Ссылки на модели
yolo_url = "https://github.com/ultralytics/assets/releases/download/v0.0.0/yolov8n.onnx"
arcface_url = "https://github.com/yakhyo/face-recognition-models/releases/download/v0.1.0/w600k_r50.onnx"

yolo_path = os.path.join(models_dir, "yolov8n.onnx")
arcface_path = os.path.join(models_dir, "w600k_r50.onnx")

def download_model(url, path):
    if not os.path.exists(path):
        print(f"Скачивается {os.path.basename(path)}...")
        try:
            urllib.request.urlretrieve(url, path)
            print(f"✅ Успешно скачан: {path}")
        except Exception as e:
            print(f"❌ Ошибка при скачивании {path}: {e}")
            print(f"Пожалуйста, скачайте вручную по ссылке: {url}")
    else:
        print(f"✅ {os.path.basename(path)} уже существует.")

download_model(yolo_url, yolo_path)
download_model(arcface_url, arcface_path)

print("Завершено!")

