#!/usr/bin/env bash
# postprod.sh — hậu kỳ thủ công cho video affiliate (ngoài app aicos).
#
#   scripts/postprod.sh latest                 # video mới nhất + job + ảnh 4K + caption
#   scripts/postprod.sh qc FILE.mp4 [--final]  # QC kỹ thuật (ffprobe/ffmpeg)
#   scripts/postprod.sh pack --product "Túi kem quilted" --final edit.mp4 \
#        [--video data/output/studio-<job>.mp4] [--sound "Tên sound"] \
#        [--artist "Artist"] [--link URL] [--music nhac.mp3] [--date YYYY-MM-DD] \
#        [--drive-root DIR] [--dry-run]
#
# Quy tắc (docs/POSTPROD_RUNBOOK.md): chỉ ĐỌC + COPY từ data/output, không
# ghi đè gì trên Drive (trùng tên → dừng, hỏi Ninh), không đụng API key,
# không đăng lên nền tảng nào.
set -euo pipefail

APP_DIR="${APP_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
DATA_DIR="${DATA_DIR:-$APP_DIR/data}"
OUT_DIR="$DATA_DIR/output"

die() { echo "DỪNG: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "thiếu '$1' (brew install ${2:-$1})"; }

# slugify "Túi Kem Quilted!" -> "tui-kem-quilted" (bỏ dấu tiếng Việt).
slugify() {
	printf '%s' "$1" | perl -CS -MUnicode::Normalize -ne '
		$_ = NFD($_); s/\p{Mn}//g; tr/\x{0111}\x{0110}/dD/;
		$_ = lc; s/[^a-z0-9]+/-/g; s/^-+|-+$//g; print'
}

latest_video() {
	local f
	f=$(ls -t "$OUT_DIR"/studio-*.mp4 2>/dev/null | head -1 || true)
	[ -n "$f" ] || die "chưa có video nào trong $OUT_DIR/studio-*.mp4 — chạy autopilot/Studio trong app trước."
	printf '%s' "$f"
}

job_id_of() { local b; b=$(basename "$1" .mp4); printf '%s' "${b#studio-}"; }

caption_of() {
	local db="$DATA_DIR/studio.db" id="$1"
	[ -f "$db" ] && command -v sqlite3 >/dev/null || return 0
	[[ "$id" =~ ^[A-Za-z0-9_-]+$ ]] || return 0
	sqlite3 -readonly "$db" "SELECT caption FROM studio_jobs WHERE id='$id';" 2>/dev/null || true
}

cmd_latest() {
	local v id work
	v=$(latest_video); id=$(job_id_of "$v"); work="$OUT_DIR/studio/$id"
	echo "Video:   $v"
	echo "Job id:  $id"
	echo "Ảnh 4K:"; ls "$work"/photo-*-4k.png 2>/dev/null | sed 's/^/  /' || echo "  (không có — job không phải chế độ ảnh?)"
	echo "Caption:"; caption_of "$id" | sed 's/^/  /'
}

# ------------------------------------------------------------------ QC
QC_FAIL=0
pass() { printf '  [PASS] %s\n' "$*"; }
warn() { printf '  [WARN] %s\n' "$*"; }
fail() { printf '  [FAIL] %s\n' "$*"; QC_FAIL=1; }

probe() { ffprobe -v error -select_streams "$2" -show_entries "$3" -of default=nw=1:nk=1 "$1" 2>/dev/null | head -1; }

cmd_qc() {
	need ffprobe ffmpeg; need ffmpeg
	local f="${1:-}" final=0
	[ -n "$f" ] || die "qc cần đường dẫn file"
	[ -f "$f" ] || die "không thấy file: $f"
	[ "${2:-}" = "--final" ] && final=1
	QC_FAIL=0
	echo "QC kỹ thuật: $f"

	local w h fps dur vbr acodec adur frames
	w=$(probe "$f" v:0 stream=width); h=$(probe "$f" v:0 stream=height)
	fps=$(probe "$f" v:0 stream=avg_frame_rate)
	dur=$(ffprobe -v error -show_entries format=duration -of default=nw=1:nk=1 "$f")
	vbr=$(probe "$f" v:0 stream=bit_rate)
	acodec=$(probe "$f" a:0 stream=codec_name)
	adur=$(probe "$f" a:0 stream=duration)
	frames=$(ffprobe -v error -select_streams v:0 -count_packets -show_entries stream=nb_read_packets -of default=nw=1:nk=1 "$f")

	[ "$w" = 1080 ] && [ "$h" = 1920 ] && pass "kích thước ${w}x${h}" || fail "kích thước ${w}x${h} (cần 1080x1920)"

	local fpsn
	fpsn=$(awk -v r="$fps" 'BEGIN{split(r,a,"/"); if (a[2]+0==0) a[2]=1; printf "%.2f", a[1]/a[2]}')
	awk -v x="$fpsn" 'BEGIN{exit !(x>=29.9 && x<=30.1)}' && pass "fps $fpsn" || fail "fps $fpsn (cần 30)"

	if awk -v d="$dur" 'BEGIN{exit !(d>=25 && d<=35)}'; then pass "thời lượng ${dur}s"
	else warn "thời lượng ${dur}s (chuẩn ~30s)"; fi

	# Khung hình rơi: số frame thực so với thời lượng × fps.
	local expect
	expect=$(awk -v d="$dur" -v r="$fpsn" 'BEGIN{printf "%d", d*r}')
	if awk -v a="$frames" -v e="$expect" 'BEGIN{exit !(a+0 >= e-3)}'; then pass "frame đủ ($frames/$expect)"
	else fail "thiếu frame ($frames/$expect) — có thể giật khung"; fi

	if [ -n "$vbr" ] && [ "$vbr" != N/A ]; then
		local mbps; mbps=$(awk -v b="$vbr" 'BEGIN{printf "%.1f", b/1e6}')
		if awk -v m="$mbps" 'BEGIN{exit !(m>=12)}'; then pass "bitrate ${mbps} Mbps"
		elif [ $final = 1 ]; then fail "bitrate ${mbps} Mbps (export CapCut cần ≥ 12 Mbps)"
		else warn "bitrate ${mbps} Mbps (bản gốc CRF — chỉ bản final mới cần ≥ 12)"; fi
	fi

	if [ -z "$acodec" ]; then
		[ $final = 1 ] && fail "không có nhạc" || warn "không có nhạc (bản gốc chưa ghép sound)"
	else
		pass "có audio ($acodec)"
		if [ -n "$adur" ] && [ "$adur" != N/A ]; then
			awk -v a="$adur" -v d="$dur" 'BEGIN{x=d-a; if(x<0)x=-x; exit !(x<=0.5)}' \
				&& pass "nhạc phủ hết video (${adur}s)" || fail "nhạc ${adur}s ≠ video ${dur}s — bị cắt cụt/hụt"
		fi
		local maxv meanv
		read -r maxv meanv < <(ffmpeg -nostats -i "$f" -vn -af volumedetect -f null - 2>&1 |
			awk '/max_volume/{m=$5} /mean_volume/{n=$5} END{print m, n}')
		if awk -v m="$maxv" 'BEGIN{exit !(m+0 >= -0.1)}'; then fail "đỉnh âm ${maxv} dB — có thể rè/clip"
		else pass "đỉnh âm ${maxv} dB"; fi
		awk -v n="$meanv" 'BEGIN{exit !(n+0 < -30)}' && warn "âm lượng TB ${meanv} dB — nhạc quá nhỏ" || pass "âm lượng TB ${meanv} dB"
	fi

	echo "  [TAY ] mặt mẫu đồng nhất · tay tự nhiên · sản phẩm đúng mẫu · không chữ/watermark/logo · nhạc khớp nhịp cắt"
	[ $QC_FAIL = 0 ] && echo "QC kỹ thuật: ĐẠT" || echo "QC kỹ thuật: KHÔNG ĐẠT — sửa trong CapCut rồi export lại."
	return $QC_FAIL
}

# ---------------------------------------------------------------- pack
drive_root() {
	local d
	for d in "$HOME"/Library/CloudStorage/GoogleDrive-*/"My Drive" "$HOME"/Library/CloudStorage/GoogleDrive-*/"Drive của tôi"; do
		[ -d "$d" ] && { printf '%s' "$d"; return; }
	done
	die "chưa đăng nhập Google Drive for desktop (không thấy ~/Library/CloudStorage/GoogleDrive-*). Mở app Google Drive, đăng nhập rồi chạy lại."
}

drive_link() {
	local id i
	for i in $(seq 1 "${DRIVE_LINK_WAIT:-15}"); do
		id=$(xattr -p 'com.google.drivefs.item-id#S' "$1" 2>/dev/null || true)
		if [ -n "$id" ] && [[ "$id" != local* ]]; then
			echo "https://drive.google.com/drive/folders/$id"; return
		fi
		sleep 2
	done
	echo "(Drive chưa đồng bộ xong — mở app Google Drive lấy link folder)"
}

cmd_pack() {
	local product="" final="" video="" sound="" artist="" link="" music="" day root="" dry=0
	day=$(date +%F)
	while [ $# -gt 0 ]; do
		case "$1" in
		--product) product="$2"; shift 2 ;;
		--final) final="$2"; shift 2 ;;
		--video) video="$2"; shift 2 ;;
		--sound) sound="$2"; shift 2 ;;
		--artist) artist="$2"; shift 2 ;;
		--link) link="$2"; shift 2 ;;
		--music) music="$2"; shift 2 ;;
		--date) day="$2"; shift 2 ;;
		--drive-root) root="$2"; shift 2 ;;
		--dry-run) dry=1; shift ;;
		*) die "tham số lạ: $1" ;;
		esac
	done
	[ -n "$product" ] || die "thiếu --product"
	[ -n "$final" ] && [ -f "$final" ] || die "thiếu --final (file export từ CapCut)"
	[[ "$day" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || die "--date phải dạng YYYY-MM-DD"
	[ -n "$video" ] || video=$(latest_video)
	[ -f "$video" ] || die "không thấy video gốc: $video"
	[ -z "$music" ] || [ -f "$music" ] || die "không thấy file nhạc: $music"

	local slug id work name dest
	slug=$(slugify "$product"); [ -n "$slug" ] || die "tên sản phẩm rỗng sau khi slugify"
	id=$(job_id_of "$video"); work="$OUT_DIR/studio/$id"
	name="${day}_${slug}"
	[ -n "$root" ] || root=$(drive_root)
	[ -d "$root" ] || die "không thấy Drive root: $root"
	dest="$root/AICOS/videos/$name"

	cmd_qc "$final" --final || die "bản final chưa qua QC kỹ thuật — không upload."

	# Danh sách copy: nguồn|tên đích
	local items=() p
	items+=("$final|${name}_final.mp4")
	items+=("$video|$(basename "$video")")
	for p in "$work"/photo-*-4k.png; do [ -f "$p" ] && items+=("$p|$(basename "$p")"); done
	[ -n "$music" ] && items+=("$music|$(basename "$music")")

	# Không ghi đè: trùng tên → dừng, để Ninh quyết.
	local conflicts=() it
	for it in "${items[@]}" "x|sound.txt" "x|caption.txt"; do
		[ -e "$dest/${it#*|}" ] && conflicts+=("$dest/${it#*|}")
	done
	[ ${#conflicts[@]} -eq 0 ] || { printf '  đã tồn tại: %s\n' "${conflicts[@]}" >&2; die "file đã có trên Drive — hỏi Ninh trước khi ghi đè (hoặc đổi --date/--product)."; }

	echo "Upload → $dest"
	for it in "${items[@]}"; do echo "  + ${it#*|}"; done
	echo "  + sound.txt"; echo "  + caption.txt"
	[ $dry = 1 ] && { echo "(dry-run: không copy gì)"; return; }

	mkdir -p "$dest"
	for it in "${items[@]}"; do cp -n "${it%%|*}" "$dest/${it#*|}"; done
	{
		echo "Sound: ${sound:-(chưa ghi)}"
		echo "Artist: ${artist:-(chưa ghi)}"
		echo "Link: ${link:-}"
		echo "Video gốc: $(basename "$video") (job $id)"
	} >"$dest/sound.txt"
	caption_of "$id" >"$dest/caption.txt"

	echo "Xong. Folder: $dest"
	echo "Link: $(drive_link "$dest")"
}

case "${1:-}" in
latest) cmd_latest ;;
qc) shift; cmd_qc "$@" ;;
pack) shift; cmd_pack "$@" ;;
slug) slugify "${2:-}"; echo ;;
*) sed -n '2,13p' "$0"; exit 2 ;;
esac
