#!/usr/bin/env python3
"""
scripts/face_swapper.py — AI Face Swap Engine for MiuNova
Hoán đổi danh tính nhân vật kênh vào video chuyển động TikTok.
Hỗ trợ cả InsightFace/ONNXRuntime và fallback OpenCV Seamless Cloning.
"""

import sys
import os
import argparse
import json
import time
import subprocess
import shutil

try:
    import cv2
    import numpy as np
except ImportError as e:
    cv2 = None
    np = None

def log(msg):
    print(f"LOG: {msg}", flush=True)

def progress(percent):
    print(f"PROGRESS: {int(percent)}", flush=True)

def check_env():
    model_dir = os.path.expanduser("~/.insightface/models")
    buffalo_dir = os.path.join(model_dir, "buffalo_l")
    required_buffalo = ("det_10g.onnx", "w600k_r50.onnx", "2d106det.onnx")
    has_buffalo = all(os.path.isfile(os.path.join(buffalo_dir, name)) for name in required_buffalo)
    inswapper_file = os.path.join(model_dir, "inswapper_128.onnx")
    has_inswapper = os.path.isfile(inswapper_file) and os.path.getsize(inswapper_file) > 500_000_000
    status = {
        "python": sys.version,
        "cv2": cv2.__version__ if cv2 else None,
        "numpy": np.__version__ if np else None,
        "insightface": False,
        "onnxruntime": False,
        "model_dir": model_dir,
        "models_ready": has_buffalo and has_inswapper,
        "can_run": bool(cv2 and np),
        "ready": False,
    }
    try:
        import onnxruntime
        status["onnxruntime"] = onnxruntime.__version__
    except ImportError:
        pass

    try:
        import insightface
        status["insightface"] = insightface.__version__
    except ImportError:
        pass

    status["ready"] = bool(
        cv2 and np and status["insightface"] and status["onnxruntime"] and status["models_ready"]
    )
    missing = []
    if not cv2 or not np:
        missing.append("OpenCV/numpy")
    if not status["insightface"] or not status["onnxruntime"]:
        missing.append("InsightFace/ONNX Runtime")
    if not has_buffalo:
        missing.append("buffalo_l model files")
    if not has_inswapper:
        missing.append("inswapper_128.onnx model")
    if missing:
        status["last_error"] = "Thiếu " + ", ".join(missing)

    return status

def get_face_detector(w=320, h=320):
    candidates = [
        'data/models/faceswap/face_detection_yunet.onnx',
        os.path.join(os.path.dirname(__file__), '..', 'data', 'models', 'faceswap', 'face_detection_yunet.onnx'),
    ]
    for c in candidates:
        if os.path.exists(c):
            try:
                return cv2.FaceDetectorYN.create(c, "", (int(w), int(h)), score_threshold=0.55, nms_threshold=0.3)
            except Exception as e:
                log(f"Lỗi khởi tạo YuNet ({e})")
    return None

def detect_faces_yunet(detector, img):
    if detector is None or img is None:
        return []
    h, w = img.shape[:2]
    detector.setInputSize((w, h))
    _, faces = detector.detect(img)
    if faces is None or len(faces) == 0:
        return []
    out = []
    for f in faces:
        x, y, fw, fh = int(f[0]), int(f[1]), int(f[2]), int(f[3])
        # Clamp to image boundaries
        x = max(0, x)
        y = max(0, y)
        fw = min(w - x, fw)
        fh = min(h - y, fh)
        if fw > 20 and fh > 20:
            score = float(f[-1])
            out.append((x, y, fw, fh, score))
    return sorted(out, key=lambda item: item[2] * item[3], reverse=True)

def align_and_blend_landmarks(src_bgr, src_pts, target_frame, detector, prev_M=None):
    """
    High-fidelity 5-point facial landmark alignment with:
    - 2D similarity transform matching head tilt, roll, and scale
    - Exponential Moving Average (EMA) temporal smoothing across frames
    - Reinhard LAB color space skin-tone harmonization to match room lighting
    - Convex-hull organic alpha feathering
    """
    if detector is None or target_frame is None or src_pts is None:
        return target_frame, prev_M

    h_t, w_t = target_frame.shape[:2]
    detector.setInputSize((w_t, h_t))
    _, dst_faces = detector.detect(target_frame)
    if dst_faces is None or len(dst_faces) == 0:
        return target_frame, prev_M

    # Get primary face sorted by area
    df = sorted(dst_faces, key=lambda item: item[2] * item[3], reverse=True)[0]
    fw, fh = df[2], df[3]
    score = df[-1]

    # Skip if face is too small or confidence too low (e.g. dancer turned around)
    if fw < 28 or fh < 28 or score < 0.50:
        return target_frame, prev_M

    dst_pts = np.float32([
        df[4:6],   # right eye
        df[6:8],   # left eye
        df[8:10],  # nose tip
        df[10:12], # right mouth corner
        df[12:14]  # left mouth corner
    ])

    M, _ = cv2.estimateAffinePartial2D(src_pts, dst_pts)
    if M is None:
        return target_frame, prev_M

    # Temporal smoothing to prevent jitter
    if prev_M is not None:
        M = 0.82 * M + 0.18 * prev_M

    # Warp source image onto target frame dimensions
    warped_src = cv2.warpAffine(src_bgr, M, (w_t, h_t), flags=cv2.INTER_LANCZOS4)

    # Compute organic convex hull of transformed landmarks
    center = np.mean(dst_pts, axis=0).astype(np.float32)
    hull = cv2.convexHull(dst_pts.astype(np.int32))
    
    # Expand hull naturally to cover facial features (eyes, nose, mouth, cheeks)
    hull_expanded = (hull - center) * 1.35 + center
    hull_expanded = hull_expanded.astype(np.int32)

    mask = np.zeros((h_t, w_t), dtype=np.uint8)
    cv2.fillConvexPoly(mask, hull_expanded, 255)
    
    # Soft Gaussian falloff
    blur_k = max(15, (int(fw * 0.15) // 2) * 2 + 1)
    mask = cv2.GaussianBlur(mask, (blur_k, blur_k), blur_k // 2)

    # Reinhard color space harmonization (match skin lighting)
    mask_bool = mask > 15
    if np.any(mask_bool):
        try:
            src_lab = cv2.cvtColor(warped_src, cv2.COLOR_BGR2LAB).astype(np.float32)
            tgt_lab = cv2.cvtColor(target_frame, cv2.COLOR_BGR2LAB).astype(np.float32)

            src_p = src_lab[mask_bool]
            tgt_p = tgt_lab[mask_bool]

            src_m, src_s = np.mean(src_p, axis=0), np.std(src_p, axis=0) + 1e-5
            tgt_m, tgt_s = np.mean(tgt_p, axis=0), np.std(tgt_p, axis=0) + 1e-5

            norm = (src_p - src_m) / src_s
            trans = np.clip(norm * tgt_s + tgt_m, 0, 255)

            src_lab[mask_bool] = trans
            color_matched = cv2.cvtColor(src_lab.astype(np.uint8), cv2.COLOR_LAB2BGR)
        except Exception:
            color_matched = warped_src
    else:
        color_matched = warped_src

    # Smooth alpha blend
    alpha = (mask.astype(np.float32) / 255.0)[:, :, np.newaxis]
    blended = (color_matched.astype(np.float32) * alpha + target_frame.astype(np.float32) * (1.0 - alpha)).astype(np.uint8)

    return blended, M

def load_and_rank_source_faces(app, source_input):
    """
    Evaluates all provided source images, scores face frontality, landmark clarity,
    and area, and returns a prioritized list of source faces with the best frontal face first.
    """
    if isinstance(source_input, str):
        paths = [p.strip() for p in source_input.split(",") if p.strip()]
    else:
        paths = list(source_input)

    candidates = []
    for pth in paths:
        if not os.path.exists(pth):
            continue
        img = cv2.imread(pth)
        if img is None:
            continue
        faces = app.get(img)
        if not faces:
            continue
        f = sorted(faces, key=lambda x: (x.bbox[2]-x.bbox[0])*(x.bbox[3]-x.bbox[1]), reverse=True)[0]
        w = f.bbox[2] - f.bbox[0]
        h = f.bbox[3] - f.bbox[1]
        area = w * h
        score = float(f.det_score)

        # Evaluate face frontality using landmark symmetry (right eye, left eye, nose tip)
        front_score = score
        pts = getattr(f, "kps", None)
        if pts is not None and len(pts) >= 3:
            d_right = float(np.linalg.norm(pts[0] - pts[2]))
            d_left = float(np.linalg.norm(pts[1] - pts[2]))
            symmetry = min(d_right, d_left) / max(d_right, d_left, 1e-4)
            # Frontal faces with high symmetry score highest
            front_score = score * 0.5 + symmetry * 0.5

        candidates.append({
            "face": f,
            "path": pth,
            "area": area,
            "det_score": score,
            "front_score": front_score,
        })

    if not candidates:
        raise ValueError(f"Không phát hiện được khuôn mặt hợp lệ trong các ảnh mẫu: {source_input}")

    # Sort candidates by frontal quality score then area
    candidates.sort(key=lambda c: (c["front_score"], c["area"]), reverse=True)
    best = candidates[0]
    log(f"✓ Đã phân tích {len(candidates)} góc ảnh mẫu — Chọn góc mặt tối ưu nhất: '{os.path.basename(best['path'])}' (độ nét: {best['det_score']:.2f}, tỷ lệ chính diện: {best['front_score']:.2f})")
    return [c["face"] for c in candidates], best["face"]

def run_insightface_swap(source_path, target_video, output_video, max_frames=0):
    log("Khởi động InsightFace AI Face Swap Engine...")
    import insightface
    from insightface.app import FaceAnalysis

    try:
        import onnxruntime as ort
        available = ort.get_available_providers()
        provs = []
        if "CoreMLExecutionProvider" in available:
            provs.append("CoreMLExecutionProvider")
        if "CUDAExecutionProvider" in available:
            provs.append("CUDAExecutionProvider")
        provs.append("CPUExecutionProvider")
    except Exception:
        provs = ['CPUExecutionProvider']

    app = FaceAnalysis(name='buffalo_l', providers=provs)
    app.prepare(ctx_id=0, det_size=(640, 640))

    model_path = os.path.expanduser("~/.insightface/models/inswapper_128.onnx")
    if not os.path.exists(model_path):
        raise ValueError("Thiếu mô hình inswapper_128.onnx; hãy cài đặt trước khi chạy production")
    try:
        swapper = insightface.model_zoo.get_model(model_path, providers=provs)
    except Exception:
        swapper = insightface.model_zoo.get_model(model_path)

    all_source_faces, source_face = load_and_rank_source_faces(app, source_path)

    cap = cv2.VideoCapture(target_video)
    if not cap.isOpened():
        raise ValueError(f"Không mở được video đích: {target_video}")

    fps = cap.get(cv2.CAP_PROP_FPS) or 25.0
    width = int(cap.get(cv2.CAP_PROP_FRAME_WIDTH))
    height = int(cap.get(cv2.CAP_PROP_FRAME_HEIGHT))
    total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))

    if max_frames > 0:
        total_frames = min(total_frames, max_frames)

    log(f"Video đầu vào: {width}x{height} @ {fps:.1f}fps ({total_frames} khung hình)")

    temp_raw_video = output_video + ".raw.mp4"
    fourcc = cv2.VideoWriter_fourcc(*'mp4v')
    out = cv2.VideoWriter(temp_raw_video, fourcc, fps, (width, height))

    frame_idx = 0
    start_time = time.time()
    try:
        while True:
            ret, frame = cap.read()
            if not ret or (max_frames > 0 and frame_idx >= max_frames):
                break

            target_faces = app.get(frame)
            if len(target_faces) > 0:
                # Swap main face
                main_face = sorted(target_faces, key=lambda f: (f.bbox[2]-f.bbox[0])*(f.bbox[3]-f.bbox[1]), reverse=True)[0]
                frame = swapper.get(frame, main_face, source_face, paste_back=True)

                # Subtle unsharp masking on face bounding box to restore crisp eyes, nose, lips and skin texture
                try:
                    bx1, by1, bx2, by2 = [int(v) for v in main_face.bbox]
                    bx1, by1 = max(0, bx1), max(0, by1)
                    bx2, by2 = min(width, bx2), min(height, by2)
                    if bx2 > bx1 + 10 and by2 > by1 + 10:
                        roi = frame[by1:by2, bx1:bx2]
                        blur = cv2.GaussianBlur(roi, (0, 0), 2.0)
                        sharpened = cv2.addWeighted(roi, 1.30, blur, -0.30, 0)
                        frame[by1:by2, bx1:bx2] = sharpened
                except Exception:
                    pass

            out.write(frame)
            frame_idx += 1

            if frame_idx % 15 == 0 or frame_idx == total_frames:
                pct = (frame_idx / max(total_frames, 1)) * 90.0
                progress(pct)
                elapsed = time.time() - start_time
                fps_proc = frame_idx / max(elapsed, 0.001)
                log(f"Đã hoán đổi khuôn mặt AI: {frame_idx}/{total_frames} khung hình ({fps_proc:.1f} fps)...")
    finally:
        cap.release()
        out.release()

    # Recombine audio with ffmpeg
    progress(95)
    log("Hòa âm và xuất video cuối cùng...")
    cmd = [
        "ffmpeg", "-y",
        "-i", temp_raw_video,
        "-i", target_video,
        "-c:v", "libx264", "-preset", "fast", "-crf", "19",
        "-c:a", "aac", "-b:a", "192k",
        "-map", "0:v:0",
        "-map", "1:a:0?",
        "-shortest",
        output_video
    ]
    subprocess.run(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
    if os.path.exists(temp_raw_video):
        os.remove(temp_raw_video)

    progress(100)
    log(f"✓ Hoàn tất AI Face Swap thành công: {output_video}")

def run_fallback_swap(source_path, target_video, output_video, max_frames=0):
    log("Sử dụng bộ xử lý Face Alignment & Seamless Cloning...")
    cap = cv2.VideoCapture(target_video)
    if not cap.isOpened():
        raise ValueError(f"Không mở được video đích: {target_video}")

    fps = cap.get(cv2.CAP_PROP_FPS) or 25.0
    width = int(cap.get(cv2.CAP_PROP_FRAME_WIDTH))
    height = int(cap.get(cv2.CAP_PROP_FRAME_HEIGHT))
    total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))

    if max_frames > 0:
        total_frames = min(total_frames, max_frames)

    log(f"Video đầu vào: {width}x{height} @ {fps:.1f}fps ({total_frames} khung hình)")

    detector = get_face_detector(width, height)
    if detector is None:
        raise ValueError("Không tìm thấy mô hình nhận diện khuôn mặt YuNet")

    # Detect face and landmarks in source images (evaluate best angle)
    paths = [p.strip() for p in source_path.split(",") if p.strip()]
    src_bgr = None
    src_pts = None
    best_score = -1.0
    for pth in paths:
        if not os.path.exists(pth):
            continue
        im = cv2.imread(pth)
        if im is None:
            continue
        im_h, im_w = im.shape[:2]
        detector.setInputSize((im_w, im_h))
        _, faces = detector.detect(im)
        if faces is not None and len(faces) > 0:
            sf = sorted(faces, key=lambda x: x[2]*x[3], reverse=True)[0]
            score = float(sf[-1])
            if score > best_score:
                best_score = score
                src_bgr = im
                src_pts = np.float32([sf[4:6], sf[6:8], sf[8:10], sf[10:12], sf[12:14]])
                log(f"✓ Đã chọn góc mặt '{os.path.basename(pth)}' (độ tin cậy: {score:.2f})")

    if src_bgr is None:
        raise ValueError(f"Không đọc được ảnh nhân vật nguồn từ: {source_path}")
        log("Cảnh báo: Không phát hiện được điểm mốc khuôn mặt nhân vật, giữ nguyên video gốc")

    temp_raw_video = output_video + ".raw.mp4"
    fourcc = cv2.VideoWriter_fourcc(*'mp4v')
    out = cv2.VideoWriter(temp_raw_video, fourcc, fps, (width, height))

    frame_idx = 0
    start_time = time.time()
    prev_M = None
    try:
        while True:
            ret, frame = cap.read()
            if not ret or (max_frames > 0 and frame_idx >= max_frames):
                break

            if src_pts is not None:
                swapped, prev_M = align_and_blend_landmarks(src_bgr, src_pts, frame, detector, prev_M)
            else:
                swapped = frame

            out.write(swapped)
            frame_idx += 1

            if frame_idx % 25 == 0 or frame_idx == total_frames:
                pct = (frame_idx / max(total_frames, 1)) * 90.0
                progress(pct)
                elapsed = time.time() - start_time
                fps_proc = frame_idx / max(elapsed, 0.001)
                log(f"Đã xử lý: {frame_idx}/{total_frames} khung hình ({fps_proc:.1f} fps)...")
    finally:
        cap.release()
        out.release()

    progress(95)
    log("Hòa âm và đóng gói MP4...")
    cmd = [
        "ffmpeg", "-y",
        "-i", temp_raw_video,
        "-i", target_video,
        "-c:v", "libx264", "-preset", "fast", "-crf", "19",
        "-c:a", "aac", "-b:a", "192k",
        "-map", "0:v:0",
        "-map", "1:a:0?",
        "-shortest",
        output_video
    ]
    subprocess.run(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
    if os.path.exists(temp_raw_video):
        os.remove(temp_raw_video)

    progress(100)
    log(f"✓ Hoàn tất xử lý khuôn mặt nhân vật: {output_video}")

def main():
    parser = argparse.ArgumentParser(description="AI Face Swap Engine for MiuNova")
    parser.add_argument("--source", type=str, help="Đường dẫn ảnh nhân vật khuôn mặt nguồn")
    parser.add_argument("--target", type=str, help="Đường dẫn video chuyển động đích")
    parser.add_argument("--output", type=str, help="Đường dẫn file video kết quả đầu ra")
    parser.add_argument("--max-frames", type=int, default=0, help="Giới hạn số frame xử lý (0 = toàn bộ)")
    parser.add_argument("--mode", type=str, default="auto", choices=["auto", "insightface", "fast"], help="Chế độ xử lý khuôn mặt")
    parser.add_argument("--strict", action="store_true", help="Production mode: require InsightFace models; never fall back")
    parser.add_argument("--check", action="store_true", help="Kiểm tra trạng thái môi trường AI")
    args = parser.parse_args()

    if args.check:
        status = check_env()
        print(json.dumps(status, indent=2))
        return

    if not args.source or not args.target or not args.output:
        parser.print_help()
        sys.exit(1)

    source_list = [p.strip() for p in args.source.split(",") if p.strip()]
    valid_sources = [p for p in source_list if os.path.exists(p)]
    if not valid_sources:
        log(f"LỖI: Không tìm thấy bất kỳ tệp ảnh nguồn nào tồn tại: {args.source}")
        sys.exit(2)

    if not os.path.exists(args.target):
        log(f"LỖI: Tệp video đích không tồn tại: {args.target}")
        sys.exit(3)

    os.makedirs(os.path.dirname(os.path.abspath(args.output)), exist_ok=True)

    status = check_env()
    if args.strict:
        if not status["ready"]:
            raise RuntimeError(status.get("last_error", "InsightFace production engine chưa sẵn sàng"))
        run_insightface_swap(args.source, args.target, args.output, args.max_frames)
        return

    # Check if InsightFace model files are fully downloaded
    has_buffalo = status.get("models_ready", False)
    has_insightface = bool(status.get("insightface") and status.get("onnxruntime"))

    use_insightface = False
    if args.mode == "insightface":
        use_insightface = True
    elif args.mode == "auto":
        use_insightface = has_buffalo and has_insightface
    else: # fast
        use_insightface = False

    try:
        if use_insightface:
            run_insightface_swap(args.source, args.target, args.output, args.max_frames)
        else:
            run_fallback_swap(args.source, args.target, args.output, args.max_frames)
    except Exception as e:
        log(f"LỖI NGHIÊM TRỌNG: {e}")
        sys.exit(4)

if __name__ == "__main__":
    main()
