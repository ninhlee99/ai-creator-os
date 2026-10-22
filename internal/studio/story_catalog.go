package studio

import (
	"fmt"
	"strings"
	"time"
)

// CuratedAudioStoryPlots chứa danh sách các cốt truyện audio YouTube kinh điển (hệ thống tự nghĩ sẵn 100%).
var CuratedAudioStoryPlots = []AudioStoryPlot{
	{
		ID:              "plot-dem-khuya-01",
		Title:           "Đêm Mưa Bên Quán Trọ Cũ",
		Genre:           "tâm lý",
		GenreLabel:      "Radio Kể Chuyện Đêm Khuya",
		DurationMinutes: 45,
		Logline:         "Một lữ khách lỡ đường trong đêm bão táp tá túc tại quán trọ ven sông và nghe câu chuyện ân nghĩa cảm động.",
		Act1:            "Đêm mưa bão mù trời, người đàn ông ướt sũng bước vào quán trọ ven sông của bà cụ già và cô cháu gái ít nói.",
		Act2:            "Bên tách trà gừng nghi ngút khói, bà cụ kể về người khách quen 10 năm trước đã gửi lại chiếc hộp gỗ bí ẩn chưa từng quay lại.",
		Act3:            "Khi người đàn ông mở chiếc hộp, bức thư cũ và chiếc vòng bạc tiết lộ sự thật: ông lão gửi hộp chính là người cha bặt vô âm tín của anh.",
		Act4:            "Bà cụ run rẩy rút ra lá thư người cha gửi gắm trước lúc lâm chung, tha thứ mọi lỗi lầm giận dỗi tuổi trẻ của đứa con thơ dại.",
		Act5:            "Trong tiếng mưa rả rích đầu hiên vắng, giọt nước mắt hối hận hòa tan gánh nặng mười năm, khép lại câu chuyện tình phụ tử thiêng liêng.",
		Message:         "Ở đời, ân nghĩa và tình thân lớn hơn mọi sóng gió thời gian.",
		SuggestedVoice:  "default",
		Soundscape:      "Tiếng mưa rơi nhẹ + Piano lofi acoustic",
	},
	{
		ID:              "plot-dem-khuya-02",
		Title:           "Chuyến Xe Buýt Cuối Cùng Trở Về Nhà",
		Genre:           "tâm lý",
		GenreLabel:      "Radio Kể Chuyện Đêm Khuya",
		DurationMinutes: 30,
		Logline:         "Những cuộc gặp gỡ định mệnh trên chuyến xe buýt đêm muộn đưa một chàng trai trẻ tìm lại ý nghĩa cuộc sống.",
		Act1:            "Chàng trai trẻ thất nghiệp ngồi gục đầu trên chuyến xe buýt tuyến cuối lúc 23h30 trong ngày đông giá rét.",
		Act2:            "Một ông lão bán vé số dạo bước lên xe, chia cho anh nửa củ khoai nướng nóng hổi và kể về chuỗi ngày cơ cực thời trẻ.",
		Act3:            "Người phụ nữ bế con nhỏ ngồi ghế sau cất tiếng hát ru ngọt ngào, làm dịu đi cái lạnh cắt da và không khí ngột ngạt trong khoang xe.",
		Act4:            "Khi xe ghé trạm cuối ngoại thành, người lái xe mỉm cười tặng anh tấm danh thiếp tuyển dụng của một xưởng mộc đang cần người học việc.",
		Act5:            "Bước xuống xe trong sương đêm, chàng trai nhận ra giữa giông bão cuộc đời, lòng tốt của những người xa lạ luôn là ngọn đèn soi lối.",
		Message:         "Đôi khi, một chút ấm áp từ người xa lạ lại đủ sức cứu rỗi cả một cuộc đời.",
		SuggestedVoice:  "default",
		Soundscape:      "Tiếng còi xe xa xăm + Giai điệu mộc mạc",
	},
	{
		ID:              "plot-truyen-ma-01",
		Title:           "Tiếng Bước Chân Sau Bờ Tre Làng",
		Genre:           "kinh dị",
		GenreLabel:      "Truyện Ma Làng Quê",
		DurationMinutes: 45,
		Logline:         "Bí mật rùng rợn về ngôi mộ vô danh dưới gốc đa đầu làng và món nợ nhân quả của phú hộ xưa.",
		Act1:            "Đêm rằm tháng bảy, chàng sinh viên về quê chịu tang ông nội thì nghe tiếng bước chân dồn dập sau rặng tre làng lúc canh ba.",
		Act2:            "Người làng cảnh báo tuyệt đối không ai được bén mảng ra gốc đa cổ thụ sau nửa đêm, nơi từng có một nấm mồ vô danh.",
		Act3:            "Cuốn nhật ký cổ bằng chữ Nôm của ông nội để lại hé lộ bí mật kinh hoàng về món nợ máu của gia tộc phú hộ xưa cách đây một thế kỷ.",
		Act4:            "Cuộc đối mặt rợn gáy dưới ánh trăng mờ nhân ảnh và lễ tạ lỗi chân thành trước vong linh oan khuất bị chôn vùi trong bóng tối lịch sử.",
		Act5:            "Khi ánh bình minh ló rạng trên rặng tre ngả bóng vàng, oán khí tiêu tan, bài học nhân quả luân hồi để lại dư âm sâu sắc ngàn đời.",
		Message:         "Gieo nhân nào gặt quả nấy, sống ngay thẳng thì quỷ thần cũng phải kính nhường.",
		SuggestedVoice:  "default",
		Soundscape:      "Tiếng gió rít đêm sương + Tiếng côn trùng vắng",
	},
	{
		ID:              "plot-truyen-ma-02",
		Title:           "Căn Nhà Bỏ Hoang Bên Bến Đò Cũ",
		Genre:           "kinh dị",
		GenreLabel:      "Truyện Ma Làng Quê",
		DurationMinutes: 30,
		Logline:         "Ngôi nhà gỗ mục nát ngã ba sông và bí ẩn về người lái đò không bao giờ nhận tiền của khách lỡ chuyến.",
		Act1:            "Đêm mưa gió mịt mùng, một vị khách lỡ độ đường ghé vào bến đò vắng và được người đàn ông bí ẩn lặng lẽ chèo thuyền đưa sang sông.",
		Act2:            "Người chèo đò đưa khách vào căn nhà gỗ mục nát sưởi ấm, kể về người thầy lang từng dốc hết gia tài cứu cả làng trong nạn đói năm xưa.",
		Act3:            "Nửa đêm tỉnh giấc, người khách nhận ra căn nhà hoàn toàn hoang phế, chiếc bếp lò đã nguội lạnh từ hàng chục năm.",
		Act4:            "Sáng hôm sau hỏi han các vị cao niên, người khách bàng hoàng khi biết người chèo đò và căn nhà ấy đã chìm vào dòng nước từ trận lũ 30 năm trước.",
		Act5:            "Tấm bia đá tri ân bên bờ sông khẳng định đạo lý: Tấm lòng nhân đức cứu người dẫu qua bao kiếp luân hồi vẫn tỏa sáng giữa nhân gian.",
		Message:         "Tấm lòng nhân ái thì dù qua bao kiếp luân hồi vẫn lưu lại phước báu nhân gian.",
		SuggestedVoice:  "default",
		Soundscape:      "Tiếng nước róc rách + Tiếng chuông chùa vẳng xa",
	},
	{
		ID:              "plot-trinh-tham-01",
		Title:           "Bức Thư Bí Mật Trong Chiếc Đồng Hồ Cổ",
		Genre:           "trinh thám",
		GenreLabel:      "Kỳ Án Trinh Thám & Giải Mã",
		DurationMinutes: 60,
		Logline:         "Vụ mất tích bí ẩn của một giáo sư già và cuộc đấu trí nghẹt thở giữa viên thám tử và kẻ giấu mặt.",
		Act1:            "Giáo sư sử học qua đời bí ẩn tại biệt thự cổ kính, để lại chiếc đồng hồ quả lắc Thụy Sĩ đứng kim đúng 3 giờ 15 phút.",
		Act2:            "Thám tử tư lần theo các ký hiệu mật mã khắc trên bánh răng đồng hồ, phát hiện bản đồ dẫn đến bộ sưu tập cổ vật bị thất lạc thời chiến.",
		Act3:            "Những cái bẫy nguy hiểm trong hầm ngầm và cuộc rượt đuổi nghẹt thở với tổ chức buôn lậu đồ cổ xuyên quốc gia.",
		Act4:            "Cú twist lật màn khi hung thủ thực sự lại chính là người học trò xuất sắc nhất mà giáo sư từng hết lòng yêu quý và tin tưởng.",
		Act5:            "Bản di chúc thực sự được công bố trước tòa án, toàn bộ bảo vật được hiến tặng cho bảo tàng quốc gia, kết thúc vụ án kinh điển.",
		Message:         "Lưới trời lồng lộng, không tội ác nào có thể che giấu mãi mãi.",
		SuggestedVoice:  "default",
		Soundscape:      "Dramatic Suspense Ambient + Tiếng tích tắc đồng hồ",
	},
	{
		ID:              "plot-nhan-sinh-01",
		Title:           "Bát Cơm Nguội Của Người Mẹ Già",
		Genre:           "triết lý",
		GenreLabel:      "Bài Học Nhân Sinh & Đạo Lý",
		DurationMinutes: 30,
		Logline:         "Câu chuyện xúc động về sự hy sinh thầm lặng của người mẹ nghèo và bài học thức tỉnh người con thành đạt.",
		Act1:            "Sau 15 năm bôn ba làm ăn thành đạt nơi phố thị, người con trai lần đầu trở về căn nhà tranh ngói cũ ven đê thăm mẹ già tóc bạc.",
		Act2:            "Bữa cơm đạm bạc với bát cơm nguội mẹ nhường con ăn trước, còn mình lặng lẽ ăn khoai sắn bên góc bếp un khói.",
		Act3:            "Người con vô tình tìm thấy chiếc túi vải cũ kỹ cất dưới đáy rương, bên trong là từng đồng tiền lẻ thấm mồ hôi mẹ dành dụm gửi anh học đại học.",
		Act4:            "Cơn đột quỵ bất ngờ của mẹ trong đêm khiến người con hoảng loạn đưa mẹ đi cấp cứu, đối mặt với sự mong manh của kiếp người.",
		Act5:            "Giọt nước mắt muộn màng bên giường bệnh và lời thì thầm của mẹ nhắc nhở: Tiền tài danh vọng không bằng một bữa cơm đoàn viên bên gia đình.",
		Message:         "Đừng đợi đến khi cha mẹ khuất núi mới nhận ra điều quý giá nhất trên đời.",
		SuggestedVoice:  "female",
		Soundscape:      "Acoustic Guitar mộc mạc + Tiếng sáo ru êm đềm",
	},
	{
		ID:              "plot-thuong-truong-01",
		Title:           "Cú Lật Kèo Phút 89",
		Genre:           "kinh doanh",
		GenreLabel:      "Thương Trường & Khởi Nghiệp",
		DurationMinutes: 45,
		Logline:         "Từ bờ vực phá sản với khoản nợ hàng chục tỷ, chàng trai 28 tuổi lội ngược dòng ngoạn mục nhờ một nguyên tắc kinh doanh bất biến.",
		Act1:            "Công ty công nghệ đứng bên bờ vực phá sản khi đối tác chiến lược đột ngột đơn phương hủy hợp đồng trước hạn chót thanh toán 48 giờ.",
		Act2:            "Áp lực khủng khiếp từ ngân hàng và nhà cung ứng, đội ngũ sáng lập chia rẽ gay gắt giữa việc tuyên bố phá sản hay cầm cố nhà cửa.",
		Act3:            "Quyết định sống còn: CEO trẻ tuổi mở toàn bộ mã nguồn và công khai minh bạch tài chính để kêu gọi sự chung tay từ cộng đồng người dùng.",
		Act4:            "Sự ủng hộ bùng nổ từ hàng ngàn khách hàng trung thành thu hút sự chú ý của quỹ đầu tư mạo hiểm hàng đầu khu vực.",
		Act5:            "Thương vụ sáp nhập lịch sử được ký kết tại phút 89, khẳng định chân lý: Chữ Tín và giá trị thực sự cho cộng đồng là vũ khí mạnh nhất trong kinh doanh.",
		Message:         "Trong kinh doanh, chữ TÍN chính là tài sản quý giá nhất.",
		SuggestedVoice:  "default",
		Soundscape:      "Inspiring Ambient Beat + Nhịp điệu dồn dập",
	},
	{
		ID:              "plot-da-su-01",
		Title:           "Huyền Thoại Thanh Gươm Dưới Lòng Hồ",
		Genre:           "lịch sử",
		GenreLabel:      "Dã Sử Huyền Huyễn",
		DurationMinutes: 60,
		Logline:         "Câu chuyện dã sử hào hùng về người tráng sĩ vô danh đứng lên bảo vệ bờ cõi biên ải trước quân thù.",
		Act1:            "Đêm mùa đông buốt giá nơi ải bắc, tiếng tù và báo động giặc dữ tràn qua biên ải, đốt phá thôn làng bình yên.",
		Act2:            "Người tráng sĩ thợ rèn nhận lời ủy thác tìm lại thanh bảo kiếm gia truyền ngâm sâu dưới đáy hồ nước thần giữa rừng thiêng.",
		Act3:            "Vượt qua thử thách sinh tử trong hang đá ngầm và trận giao tranh khốc liệt chống lại toán thích khách tinh nhuệ.",
		Act4:            "Bảo kiếm xuất thế tỏa ánh hàn quang lẫm liệt, vị tráng sĩ hiệu triệu hào kiệt bốn phương tập hợp dưới cờ khởi nghĩa cứu dân.",
		Act5:            "Trận phục kích quyết định tại hẻm núi bẻ gãy ý chí quân xâm lược, thanh gươm được quy phục về lòng hồ, ngàn năm giữ yên cõi bờ.",
		Message:         "Tinh thần bảo vệ non sông ngàn đời là dòng máu chảy trong tim mỗi người con đất Việt.",
		SuggestedVoice:  "default",
		Soundscape:      "Đàn tranh + Tiếng trống trận hào hùng vang dội",
	},
	{
		ID:              "plot-tinh-cam-01",
		Title:           "Hẹn Ước Dưới Mưa Năm 17 Tuổi",
		Genre:           "tình cảm",
		GenreLabel:      "Tình Cảm Nam Nữ & Ngôn Tình Đô Thị",
		DurationMinutes: 45,
		Logline:         "Sau 8 năm hiểu lầm và xa cách, định mệnh sắp đặt cuộc hội ngộ cay đắng nhưng vỡ òa giữa giám đốc tập đoàn và cô thư ký từng là mối tình đầu thời cấp ba.",
		Act1:            "Chàng trai nghèo và cô tiểu thư từng có mối tình học trò đẹp như tranh vẽ dưới gốc phượng già trường huyện, trước khi biến cố gia đình khiến cô đột ngột biến mất không một lời từ biệt.",
		Act2:            "8 năm sau, anh trở thành tổng tài lạnh lùng quyền lực, còn cô xuất hiện trong buổi phỏng vấn thư ký với bộ dạng tiều tụy gánh trên vai món nợ viện phí cho mẹ.",
		Act3:            "Những lạnh lùng xa cách nơi công sở không che giấu được những quan tâm vụng về và ký ức thanh xuân chưa từng phai nhạt.",
		Act4:            "Bức thư tình bị thất lạc 8 năm trước được tìm thấy trong tập tài liệu cũ, hóa giải mọi hiểu lầm và tổn thương quá khứ.",
		Act5:            "Dưới cơn mưa đầu mùa bên hiên quán cũ, hai bàn tay lại tìm về nhau, viết tiếp cái kết viên mãn cho câu chuyện tình thanh xuân dang dở.",
		Message:         "Chân tình nếu đủ sâu đậm, dù đi một vòng bão giông rồi cũng sẽ tìm về đúng người.",
		SuggestedVoice:  "warm-female",
		Soundscape:      "Piano ballad lãng mạn + Tiếng mưa rơi êm dịu",
	},
	{
		ID:              "plot-tinh-cam-02",
		Title:           "Bức Tranh Chưa Kịp Đặt Tên",
		Genre:           "tình cảm",
		GenreLabel:      "Tình Cảm Nam Nữ & Ngôn Tình Đô Thị",
		DurationMinutes: 30,
		Logline:         "Mối tình ngọt ngào nhưng đầy trắc trở giữa chàng họa sĩ khiếm thị và cô gái làm việc tại phòng triển lãm tranh nghệ thuật.",
		Act1:            "Tại góc quán cà phê sách ven hồ Tây, cô gái tình cờ quen một chàng trai mù có tài vẽ tranh sơn dầu tuyệt mỹ qua cảm nhận xúc giác và mùi hương.",
		Act2:            "Những ngày tháng cùng nhau rong ruổi qua từng con phố Hà Nội đã nhen nhóm ngọn lửa tình yêu sâu sắc, nhưng căn bệnh hiểm nghèo khiến cô gái chọn cách âm thầm rút lui.",
		Act3:            "Khi anh được chỉ định phẫu thuật mắt có cơ hội phục hồi thị lực, cô gái phát hiện căn bệnh nan y của mình và lặng lẽ rời xa.",
		Act4:            "Ngày tháo băng mắt nhìn thấy ánh sáng, chàng trai bàng hoàng nhận ra bức tranh tuyệt mỹ cô gửi lại chính là chân dung của chính cô.",
		Act5:            "Cuộc hội ngộ nghẹn ngào tại vườn hoa cúc họa mi năm nào, tình yêu đích thực đã vượt qua mọi rào cản số phận để ở bên nhau trọn đời.",
		Message:         "Tình yêu đích thực không nhìn bằng mắt, mà thấu hiểu bằng cả trái tim.",
		SuggestedVoice:  "default",
		Soundscape:      "Acoustic Guitar + Violin da diết sâu lắng",
	},
}

// GetCuratedAudioStories trả về danh sách cốt truyện audio mẫu theo thể loại.
func GetCuratedAudioStories(genre string) []AudioStoryPlot {
	genre = strings.ToLower(strings.TrimSpace(genre))
	if genre == "" || genre == "all" {
		return append([]AudioStoryPlot(nil), CuratedAudioStoryPlots...)
	}
	var out []AudioStoryPlot
	for _, p := range CuratedAudioStoryPlots {
		if strings.EqualFold(p.Genre, genre) || strings.Contains(strings.ToLower(p.GenreLabel), genre) {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return append([]AudioStoryPlot(nil), CuratedAudioStoryPlots...)
	}
	return out
}

// DefaultAudioStoryTopic trả về 1 chủ đề cốt truyện audio hấp dẫn theo thể loại nếu người dùng để trống.
func DefaultAudioStoryTopic(genre string) string {
	plots := GetCuratedAudioStories(genre)
	if len(plots) > 0 {
		return plots[0].Title + " - " + plots[0].Logline
	}
	return "Đêm Mưa Bên Quán Trọ Cũ - Câu chuyện cảm động về ân nghĩa cuộc đời"
}

// GenerateUniqueAudioStory sinh ra 1 cốt truyện audio YouTube trường thiên (30 - 60 phút)
// theo đúng chủ đề người dùng cài đặt, với cấu trúc 5 hồi kịch tính, lôi cuốn và không bao giờ trùng lặp.
func GenerateUniqueAudioStory(customTopic, genre string, durationMinutes int) AudioStoryPlot {
	if durationMinutes <= 0 {
		durationMinutes = 30
	}
	if durationMinutes < 20 {
		durationMinutes = 30
	}
	if durationMinutes > 90 {
		durationMinutes = 60
	}

	genre = strings.ToLower(strings.TrimSpace(genre))
	if genre == "" {
		genre = "tâm lý"
	}

	words := durationMinutes * 150 // chuẩn 150 từ/phút đọc audio

	// Danh sách bối cảnh độc bản
	settings := []string{
		"Quán trọ ven sông bên bến đò xưa trong đêm mưa bão mù mịt",
		"Căn nhà cổ rêu phong ba gian nép mình sâu trong con ngõ nhỏ phố cổ",
		"Chuyến tàu đêm Thống Nhất chở những mảnh đời tha hương về quê đón Tết",
		"Trạm dừng chân heo hút trên đỉnh đèo mờ sương lạnh buốt",
		"Xóm trọ nghèo ngoại thành nơi cưu mang những số phận bươn chải nhọc nhằn",
		"Tiệm may áo dài gia truyền ba đời bên dòng kênh lịch sử",
		"Ngôi làng chài ven biển miền Trung kiên cường sau cơn bão lớn",
		"Căn biệt thự gỗ cổ kính ẩn mình giữa rừng thông sương giăng",
		"Ngôi chùa cổ thanh tịnh trên sườn núi nơi lữ khách tìm lại sự bình yên",
		"Khu chợ nổi miền sông nước với những nghĩa cử hào hiệp ấm lòng",
	}

	// Danh sách nhân vật chính
	protagonists := []string{
		"một người đàn ông trung niên mang vết sẹo ký ức và chiếc vali bí mật",
		"người thợ thủ công già câm lặng với đôi bàn tay chai sần tài hoa",
		"cô gái trẻ rời thành phố phồn hoa về lại quê xưa tìm câu trả lời cho biến cố cuộc đời",
		"bác tài xế xe tải đường dài chuyên chở những chuyến hàng đêm qua miền đất lạ",
		"cậu thanh niên nghèo tha hương mang theo lời dặn thiêng liêng của cha mẹ",
		"người thương gia từng trải qua bờ vực phá sản tìm về cội nguồn tạ lỗi",
		"cụ bà bán quán nước chè chén đầu đình lưu giữ bí mật 40 năm của cả làng",
		"người thợ làm gốm bền bỉ lưu giữ hồn đất qua bao thăng trầm thời cuộc",
	}

	// Danh sách cú twist bất ngờ (Plot Twist)
	twists := []string{
		"Người khách lạ tá túc năm xưa chính là người anh trai thất lạc hơn 20 năm của gia đình.",
		"Chiếc hộp gỗ cũ kỹ tưởng chừng rỗng không lại cất giữ cuốn sổ nợ đã được người cha trả hết bằng cả tính mạng.",
		"Ân nhân cứu mạng gia tộc năm xưa lại chính là người từng bị hàm oan mà dòng họ hắt hủi.",
		"Kẻ thù tưởng chừng tàn nhẫn thực chất lại âm thầm gánh chịu tiếng xấu để bảo vệ di sản của người mẹ.",
		"Toàn bộ chuỗi biến cố bí ẩn xảy ra chỉ để thử lòng trung hiếu và hoàn thành lời thề son sắt ba mươi năm trước.",
		"Bức thư giấu dưới đáy lư hương hé lộ bí mật chấn động về thân thế thực sự của người kế thừa.",
	}

	// Danh sách thông điệp nhân sinh sâu sắc
	messages := []string{
		"Ở đời, ân nghĩa và lòng hiếu thảo chính là ngọn đèn soi sáng qua mọi đêm đen giông bão.",
		"Tiền bạc danh vọng trôi qua như mây khói, chỉ có tình thân và sự bao dung mới ở lại muôn đời.",
		"Mỗi bước ngoặt định mệnh đều là bài học để con người ta học cách buông bỏ hận thù và trân trọng hiện tại.",
		"Sống ngay thẳng, thiện lương thì dẫu phong ba bão táp, quỷ thần cũng phải kiêng nhường.",
		"Một chút ấm áp sẻ chia giữa người với người có thể cứu rỗi cả một linh hồn lạc lối.",
	}

	// Danh sách câu Hook 3 giây giữ chân người nghe (25 mẫu, 5 nhóm thể loại)
	hooks := []string{
		// --- Mystery/Suspense (Bí ẩn / Hồi hộp) ---
		"Nửa đêm hôm ấy, khi tiếng sấm vừa rạch ngang bầu trời, một tiếng gõ cửa dồn dập vang lên làm rung chuyển cả căn nhà cũ...",
		"Có những bí mật nếu không được phơi bày trong đêm nay, có lẽ người ta sẽ mang theo cả mối ân oán xuống mồ...",
		"Người đàn ông lạ mặt bước vào quán lúc 2 giờ sáng, đặt lên bàn một bọc vải đẫm nước mưa và run rẩy nói một câu...",
		"Trước khi nhắm mắt, ông lão chỉ thì thầm đúng một câu khiến tất cả con cháu trong nhà đều chết lặng...",
		"Đúng 12 giờ đêm, chiếc chuông đồng cổ xưa bỗng nhiên tự gióng lên 3 hồi chuông lạnh toát sống lưng...",
		// --- Emotional/Touching (Cảm xúc / Chạm lòng) ---
		"Bà tôi nắm chặt tay tôi lần cuối và nói: 'Cháu à, có một điều bà giấu cháu suốt 30 năm nay...'",
		"Khi tôi mở chiếc hộp thiếc cũ kỹ mẹ để lại, bên trong là một bức thư chưa bao giờ được gửi đi...",
		"Người đàn ông ngồi khóc một mình trên ghế đá công viên lúc 4 giờ sáng. Tôi không biết rằng cuộc đời tôi sẽ thay đổi hoàn toàn kể từ khoảnh khắc ấy...",
		"Mẹ tôi chưa bao giờ kể về cha. Cho đến ngày hôm ấy, khi tôi tìm thấy tấm ảnh cũ nhàu nát trong túi áo bà...",
		"Tiếng khóc của đứa trẻ sơ sinh vang lên giữa đêm đông giá buốt ngoài cổng chùa. Sư thầy mở cửa và nhìn thấy một thứ khiến ngài chấn động...",
		// --- Horror/Creepy (Kinh dị / Rùng rợn) ---
		"Từ đêm đó, cứ đúng 3 giờ sáng, tiếng bước chân lại vang lên trên tầng áp mái. Vấn đề là... căn nhà này chỉ có một tầng...",
		"Người ta nói rằng ai đi ngang qua cây cầu đá cũ sau nửa đêm sẽ nghe thấy tiếng ru con. Tôi không tin, cho đến khi chính tôi nghe thấy...",
		"Con bé hàng xóm chỉ tay về góc tối và nói: 'Chú ơi, sao bà kia cứ đứng nhìn chú hoài vậy?' Tôi quay lại. Không có ai cả...",
		"Chiếc gương cổ trong căn biệt thự bỏ hoang không phản chiếu bóng tôi. Nó phản chiếu một người đàn bà mặc áo trắng đang đứng ngay sau lưng tôi...",
		"Đêm ấy gió rít từng cơn ngoài bờ tre, ông nội bảo tôi: 'Dù nghe thấy gì, tuyệt đối không được mở cửa.' Rồi tiếng gọi tên tôi vang lên, bằng chính giọng ông nội...",
		// --- Business/Success (Kinh doanh / Thành công) ---
		"Hôm ấy tôi đứng trước tòa nhà công ty với đúng 50 ngàn trong túi và một khoản nợ 3 tỷ đồng. Không ai tin rằng 2 năm sau, mọi thứ sẽ lật ngược hoàn toàn...",
		"Đối tác lớn nhất hủy hợp đồng ngay trước ngày ký kết. Cả công ty nhìn tôi chờ đợi. Và tôi đã đưa ra quyết định điên rồ nhất đời mình...",
		"Người đàn ông bán vé số dạo năm ấy đã dạy tôi một bài học mà cả Harvard cũng không có trong giáo trình...",
		"Tôi bị sa thải đúng ngày sinh nhật tuổi 30. Nhưng đó lại là ngày đẹp nhất cuộc đời tôi. Để tôi kể cho bạn nghe vì sao...",
		"Khi cả thế giới quay lưng, chính kẻ thù cũ lại chìa tay ra cứu tôi. Và cái giá phải trả cho lòng tốt ấy... không hề rẻ...",
		// --- Romance/Love (Tình cảm / Ngôn tình) ---
		"Anh ấy bước vào đời tôi đúng lúc tôi đã quyết định không bao giờ yêu ai nữa. Và mọi thứ bắt đầu từ một chiếc ô trong cơn mưa tầm tã...",
		"Tôi và anh hẹn gặp lại sau 5 năm tại quán cà phê góc phố cũ. Anh đến đúng giờ. Nhưng bên cạnh anh là một người phụ nữ khác...",
		"Bức thư tình cuối cùng anh viết cho tôi chỉ có đúng 8 chữ. Nhưng 8 chữ ấy đã khiến tôi khóc suốt cả đêm...",
		"Cô ấy là người yêu cũ của anh trai tôi. Và tôi... đã yêu cô ấy từ cái nhìn đầu tiên mà không dám nói...",
		"Đám cưới sắp diễn ra trong 3 ngày nữa. Nhưng tối hôm đó, tôi nhận được tin nhắn từ một số lạ: 'Em xin lỗi, chú rể không phải là người anh nghĩ đâu...'",
	}

	now := time.Now().UnixNano()
	sIdx := int(now % int64(len(settings)))
	pIdx := int((now / 7) % int64(len(protagonists)))
	tIdx := int((now / 13) % int64(len(twists)))
	mIdx := int((now / 19) % int64(len(messages)))
	hIdx := int((now / 23) % int64(len(hooks)))

	topicClean := strings.TrimSpace(customTopic)
	var title string
	if topicClean != "" {
		title = topicClean
	} else {
		switch genre {
		case "tình cảm", "tinh_cam", "ngôn tình":
			title = "Hẹn Ước Dưới Mưa Năm 17 Tuổi - Mối Tình Đầu Định Mệnh"
		case "kinh dị":
			title = "Tiếng Bước Chân Sau Bờ Tre Làng - Bí Mật Ngôi Mộ Vô Danh"
		case "trinh thám":
			title = "Vụ Mất Tích Kỳ Bí Dưới Đêm Mưa Lạnh"
		case "kinh doanh":
			title = "Cuộc Lội Ngược Dòng Của Kẻ Phá Sản"
		default:
			title = fmt.Sprintf("Ký Ức Bên %s - Chuyện Tình & Nghĩa Cuộc Đời", settings[sIdx])
		}
	}

	genreLabel := "Radio Kể Chuyện Đêm Khuya"
	switch genre {
	case "tình cảm", "tinh_cam", "ngôn tình":
		genreLabel = "Tình Cảm Nam Nữ & Ngôn Tình Đô Thị"
	case "kinh dị":
		genreLabel = "Truyện Kinh Dị & Tâm Linh Làng Quê"
	case "trinh thám":
		genreLabel = "Kỳ Án Trinh Thám & Giải Mã"
	case "kinh doanh":
		genreLabel = "Thương Trường & Khởi Nghiệp Bất Khả Thi"
	case "triết lý":
		genreLabel = "Bài Học Nhân Sinh & Đạo Lý Sâu Sắc"
	}

	plotID := fmt.Sprintf("story-%s-%d", genre, now%100000)

	return AudioStoryPlot{
		ID:              plotID,
		Title:           title,
		Genre:           genre,
		GenreLabel:      genreLabel,
		DurationMinutes: durationMinutes,
		WordsCount:      words,
		Hook3s:          hooks[hIdx],
		Logline:         fmt.Sprintf("Câu chuyện trường thiên %d phút: Cuộc hội ngộ định mệnh của %s tại %s, hé mở %s", durationMinutes, protagonists[pIdx], settings[sIdx], twists[tIdx]),
		Act1:            fmt.Sprintf("Mở màn (0-%d phút): %s bối cảnh được thiết lập tại %s. Sự xuất hiện của %s mở ra những uẩn khúc chưa từng được giải đáp.", durationMinutes/5, hooks[hIdx], settings[sIdx], protagonists[pIdx]),
		Act2:            fmt.Sprintf("Thắt nút (%d-%d phút): Những manh mối kỳ lạ dần hé lộ qua từng kỷ vật cũ. Mối xung đột ngấm ngầm giữa quá khứ và hiện tại bắt đầu đẩy các nhân vật vào thế khó.", durationMinutes/5, (durationMinutes*2)/5),
		Act3:            fmt.Sprintf("Cao trào (%d-%d phút): Đỉnh điểm kịch tính khi bí mật lớn nhất bị đặt trước ranh giới phơi bày. Mọi sự nghi ngờ và tổn thương dồn nén nhiều năm bùng nổ nghẹt thở.", (durationMinutes*2)/5, (durationMinutes*3)/5),
		Act4:            fmt.Sprintf("Bước ngoặt (%d-%d phút): Cú twist bất ngờ xoay chuyển hoàn toàn góc nhìn của người nghe: %s", (durationMinutes*3)/5, (durationMinutes*4)/5, twists[tIdx]),
		Act5:            fmt.Sprintf("Mở nút & Lắng đọng (%d-%d phút): Mọi ân oán được cởi bỏ bằng sự thấu hiểu và giọt nước mắt hối hận. Thông điệp đọng lại: %s", (durationMinutes*4)/5, durationMinutes, messages[mIdx]),
		Message:         messages[mIdx],
		SuggestedVoice:  "default",
		Soundscape:      "Tiếng mưa rơi đêm khuya + Piano lofi acoustic mộc mạc",
	}
}
