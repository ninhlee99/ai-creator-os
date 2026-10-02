package accesstrade

// KeySetting là key lưu access_key trong bảng settings của ledger.
// Định nghĩa ở đây (package accesstrade) để cả web và automation dùng
// chung một nguồn — đổi key ở UI có hiệu lực ngay, không restart.
const KeySetting = "at.access_key"
