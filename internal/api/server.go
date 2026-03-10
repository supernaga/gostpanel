package api

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/supernaga/gost-panel/internal/config"
	"github.com/supernaga/gost-panel/internal/model"
	"github.com/supernaga/gost-panel/internal/notify"
	"github.com/supernaga/gost-panel/internal/service"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

//go:embed all:dist
var staticFS embed.FS

type Server struct {
	svc          *service.Service
	cfg          *config.Config
	router       *gin.Engine
	loginLimiter *RateLimiter
	audit        *AuditLogger
	wsHub        *WSHub
	// API rate limiters
	globalAPILimiter *APIRateLimiter
	writeAPILimiter  *APIRateLimiter
	agentLimiter     *APIRateLimiter
}

func NewServer(svc *service.Service, cfg *config.Config) *Server {
	if !cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	// 缂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾鐎规洘鍔欓幃婊堟嚍閵夈儲鐣遍梻浣稿閸嬪懎煤閺嶎厼鍑犲〒姘ｅ亾闁哄本娲熷畷鐓庘攽閹邦厜褔姊洪幖鐐测偓鏇㈡晝閵夆晛桅闁告洦鍠氶悿鈧梺鍦亾閸撴碍绂掓總鍛婂仩婵ê宕弸娑欐叏婵犲嫮甯涢柟宄版嚇閹兘骞嶉淇卞仭缂傚倸鍊风粈渚€藝娴兼潙绠伴柟鎯版缁犳牠鏌ｉ幇闈涘幍闁稿鎳橀弻娑㈠箛閳轰礁顫呴悶?(闂傚倸鍊搁崐鎼佸磹閹间礁纾归柟闂寸閻ゎ喗銇勯幇鈺佺労闁搞倖娲熼弻娑㈩敃閿濆棗顦╅梺?SPA 闂傚倸鍊峰ù鍥х暦閸偅鍙忕€规洖娲︽刊浼存煥閺囩偛鈧悂宕归崒鐐寸厵闁诡垳澧楅ˉ澶愭煛鐎ｂ晝绐旈柡灞剧☉铻栭柛鎰╁妼婢瑰姊洪幖鐐测偓鏍€冮崱娑樜﹂柛鏇ㄥ枤閻も偓闂佸湱鍋撻幆灞轿涢悙鐢电＝?
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	corsConfig := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}

	if len(cfg.AllowedOrigins) > 0 {
		corsConfig.AllowOrigins = cfg.AllowedOrigins
	} else if cfg.Debug {
		// 闂傚倸鍊峰ù鍥х暦閸偅鍙忛柟缁㈠櫘閺佸嫰鏌涘☉娆愮稇闁汇値鍣ｉ弻鐔煎箚閺夊晝鎾绘煕鎼粹槄鏀婚柕鍥у瀵粙鈥﹂幋婵囶啌闂備胶绮濠氬储瑜旈幃妯虹暦閸ワ絽浜鹃悷娆忓缁€鈧┑鐐茬湴閸旀垿濡存笟鈧畷銊р偓娑櫱氶幏娲⒑閸︻収鐒炬繛瀵稿厴閸╁﹪寮撮姀锛勫幘閻熸粎澧楃敮鐐烘偩閾忓厜鍋撶憴鍕８闁搞劋绮欓悰顔锯偓锝庡枟閸嬫劙鏌涢幇顒佲枙闁?localhost
		corsConfig.AllowOriginFunc = func(origin string) bool {
			return strings.HasPrefix(origin, "http://localhost") ||
				strings.HasPrefix(origin, "http://127.0.0.1") ||
				strings.HasPrefix(origin, "https://localhost") ||
				strings.HasPrefix(origin, "https://127.0.0.1")
		}
	} else {
		// 闂傚倸鍊搁崐鐑芥倿閿曞倹鍎戠憸鐗堝笒閸ㄥ倸鈹戦悩瀹犲缂佹劖顨堥埀顒€绠嶉崕鍗炍涢鐐╂灁濞寸姴顑嗛悡鏇㈡煏婢舵ê鏋ゅù鐘崇矒閺岋綁寮崒姘粯闂佽绻愮壕顓㈠焵椤掑喚娼愭繛鍙夌墪鐓ら柕濞炬杺閳ь兛绶氬畷銊р偓娑櫱氶幏娲⒑閸︻収鐒炬繛瀵稿厴閸╁﹪寮撮姀锛勫幗濡炪倖鎸嗛崘鈺冧壕闂備礁鎲＄敮妤冩暜濡ゅ懎绠氶柡鍐ㄧ墕鎯熼梺闈涳紡鐏炴嫎婵囩節閻㈤潧浠╅柟娲讳簽瀵板﹪宕稿Δ浣镐槐闂佸搫鍟悧鍡涙偪椤曗偓閺屾稑鈽夊Ο鍏兼喖闂佺粯鎸诲ú姗€骞堥妸銉建闁糕剝顨呴～顏勨攽閻橆偄浜鹃梺瑙勵問閸犳氨澹曢崗绗轰簻妞ゆ挾鍠庨悘銉╂煛鐎ｃ劌鈧繈寮婚悢鍓叉Ч閹肩补妾ч弸鍛存⒑閸濆嫯顫﹂柛鏂块叄閸┾偓妞ゆ帒锕︾粔鐢告煕閹炬潙鍝虹€?
		corsConfig.AllowOriginFunc = func(origin string) bool {
			// 闂傚倸鍊搁崐鐑芥倿閿曞倹鍎戠憸鐗堝笒閸ㄥ倸鈹戦悩瀹犲缂佹劖顨堥埀顒€绠嶉崕鍗炍涢鐐╂灁濞寸姴顑嗛悡鏇㈡煏婢舵ê鏋涘褜鍨堕弻娑氣偓锝庡亞缁犳牠鏌曢崶褍顏€殿喕绮欐俊鎼佸Ψ閵夛妇妲楅梻鍌欑閹碱偄螞濞嗘挸钃熼柕濞炬櫔缂嶆牗淇婇妶鍌氫壕闂佸疇妫勯ˇ顖炲煝鎼粹垾鐔兼偂鎼粹寬顏勨攽閿涘嫬浜奸柛濠冪墵楠炴劙骞栨担娴嬪亾閿曞倹鍊婚柣锝呰嫰缁侊箓妫呴銏″缂佸鍨圭划濠氬箮閼恒儳鍘搁梺鍛婂姂閸斿矂鍩€椤掑偆鐒鹃柣锝囧厴椤㈡棃宕奸悢鍝勫箺闂備胶绮敋闁告ɑ鐗犻幊婊堫敂閸喓鍘遍梺鍝勫€藉▔鏇㈡倿閹间焦鐓欐い鏂垮帨閸嬫捇寮妷锔锯偓濠氭⒑閻熼偊鍤熼柛瀣洴閹灚瀵肩€涙ǚ鎷绘繛杈剧到閹芥粍绂掗姀掳浜滈柟瀛樼箖閹兼劙鏌?
			return false
		}
	}

	r.Use(cors.New(corsConfig))

	SetWSOrigins(cfg.AllowedOrigins, cfg.Debug)

	s := &Server{
		svc:              svc,
		cfg:              cfg,
		router:           r,
		loginLimiter:     NewRateLimiter(5, time.Minute, 5*time.Minute), // 濠电姷鏁告慨鐢割敊閺嶎厼闂い鏍ㄧ矊缁躲倝鏌ｉ敐鍛拱鐎规洘鐓￠弻鐔兼偋閸喓鍑￠梺鎼炲妼閸婂潡鐛弽顬ュ酣顢楅埀顒佷繆娴犲鐓?濠电姷鏁告慨鐑藉极閹间礁纾绘繛鎴烆焸濞戞矮娌弶鐐插皡缂嶄礁鐣烽悢纰辨晣闁绘洑鐒︾紞妤佺節閻㈤潧鈻堟繛浣冲洦鍋嬮柛娑樼摠閸婂灝鈹戦悩鎻掆偓鍫曞焵椤掍礁绗掓い顐ｇ箞閺佹劙宕ㄩ鈧ˉ姘舵⒒閸屾瑦绁伴柕鍡忓亾闂?闂傚倸鍊搁崐椋庣矆娓氣偓楠炲鏁嶉崟顒佹闂佸湱鍎ら崵锕€鈽夊Ο閿嬫杸闁诲函缍嗘禍鐐侯敊?
		audit:            NewAuditLogger(svc),
		wsHub:            NewWSHub(),
		globalAPILimiter: NewAPIRateLimiter(200, time.Minute),
		writeAPILimiter:  NewAPIRateLimiter(30, time.Minute),
		agentLimiter:     NewAPIRateLimiter(60, time.Minute),
	}

	// 闂傚倸鍊峰ù鍥х暦閸偅鍙忕€规洖娲ㄩ惌鍡椕归敐鍫綈婵炲懐濮撮湁闁绘ê妯婇崕鎰版煕鐎ｅ吀閭柡灞剧洴閸╁嫰宕橀浣割潓闂備胶顭堟鎼佹儗閸岀偛绠栫憸鐗堝笒閻愬﹥銇勮箛鎾愁伀婵絻鍨荤槐鎾存媴閸撳弶笑濠电偛鐨烽埀顒佸墯濞兼牕鈹戦悩瀹犲缂佲偓鐎ｎ偁浜滈柟鍝勬娴滅偓绻濆▓鍨灈闁绘绻掑Σ鎰板箳閹惧墎鎳濇繝鐢靛Т鐎氼噣寮抽妶澶嬪€甸悷娆忓缁€鈧梺缁樼墪閵堟悂鐛崘銊㈡瀻闁规儳纾ˇ顓熺箾鏉堝墽绉い鏇熺墵瀹曨垶鍩€椤掑嫭鈷掗柛灞剧懅閸斿秹鎮楃粭娑樻噽閻瑩鏌熸潏楣冩闁稿孩顨呴妴鎺戭潩閿濆懍澹曟俊鐐€戦崹娲偡閳轰胶鏆﹂柛顐ｆ处閺佸倿鏌涚仦鍓ф噭闁活偄绻樺缁樻媴閸涘﹥鍎撳┑鐐跺皺婵潙宓勯梺鍛婁緱閸欏酣鎮炴禒瀣厱妞ゆ劧绲剧粈鍐煕婵犲倻鍩ｉ柣鎿冨亰瀹曟儼顦抽柟鐧哥悼缁辨帡骞囬鐕佹闂佸搫鐭夌换婵嬪春閳ь剚銇勯幒鎴濐仾闁哄懏褰冮…璺ㄦ崉娓氼垰鍓伴梺?IP
	s.loginLimiter.SetOnBlockCallback(func(ip string, attempts int) {
		s.svc.LogOperation(0, "system", "security", "ip_block", 0,
			fmt.Sprintf("IP blocked due to excessive login attempts: %s (%d attempts)", ip, attempts),
			ip, "rate_limiter", "success")
	})

	// Start WebSocket hub
	go s.wsHub.Run()

	s.svc.InitDefaultSiteConfigs()

	s.setupRoutes()

	return s
}

func (s *Server) setupRoutes() {
	s.router.Use(PrometheusMiddleware())

	// Prometheus 闂傚倸鍊搁崐椋庣矆娴ｉ潻鑰块梺顒€绉埀顒婄畵瀹曠厧顭垮┑鍥ㄣ仢闁轰礁鍟村畷鎺戔槈濡懓鍤遍梻鍌欑劍鐎笛呮崲閸屾娲Ω閳轰胶顢呭┑顔斤供閸樿绂嶅鍫熺厸鐎广儱鍟俊鍧楁煟椤撶儑宸ラ棁?(闂傚倸鍊搁崐鎼佸磹閹间礁纾圭紒瀣紩濞差亝鍋愰悹鍥皺閿涙盯姊洪悷鏉库挃缂侇噮鍨跺畷鎴︽晸閻樺磭鍘搁梺鎼炲劘閸斿酣銆傞懠顒傜＜濞达絽鎼。濂告煏閸パ冾伃妤犵偞锕㈤幃鐑藉级閹稿骸鍔掗梻?
	s.router.GET("/metrics", s.authMiddleware(), MetricsHandler())

	// API 闂傚倸鍊峰ù鍥х暦閸偅鍙忕€规洖娲︽刊浼存煥閺囩偛鈧悂宕归崒鐐寸厵闁诡垳澧楅ˉ澶愭煛?
	api := s.router.Group("/api")
	{
		// 闂傚倸鍊搁崐鐑芥嚄閸洍鈧箓宕奸妷顔芥櫈闂佸憡鍔﹂崰鏍閸ф鈷戞い鎺嗗亾缂佸顕划濠氬冀椤愮喎浜炬鐐茬仢閸旀岸鏌熼崣澹濐亪鍩ユ径鎰潊闁绘﹢娼ф慨锔戒繆閻愵亜鈧牜鏁幒鏂哄亾濮樼厧澧撮柟?(闂傚倸鍊搁崐鐑芥嚄閸洍鈧箓宕煎婵堟嚀椤繄鎹勯搹璇℃Ф婵犵數鍋涘Λ娆撳垂閸洩缍?
		api.GET("/health", s.healthCheck)

		// 闂傚倸鍊搁崐鐑芥嚄閸洍鈧箓宕煎婵堟嚀椤繄鎹勯搹璇℃Ф婵犵數鍋涘Λ娆撳垂閸洩缍栭柡鍥ュ灪閻撴瑩寮堕崼婵嗏挃闁伙綀浜槐鎺楁偐閸愭祴鏋欓梺鍝勬湰濞叉ê顕ラ崟顖氶唶婵犻潧妫楅～鎾剁磽?(闂傚倸鍊烽悞锕傛儑瑜版帒鏄ラ柛鏇ㄥ灠閸ㄥ倿鏌ｉ敐鍛伇濞戞挸绉归弻鐔兼倷椤掆偓婢ь垶鏌ｆ惔锛勫煟闁哄本绋戦埞鎴﹀幢濡炵粯鐏?
		api.POST("/login", RateLimitMiddleware(s.loginLimiter), s.login)
		api.POST("/login/2fa", RateLimitMiddleware(s.loginLimiter), s.login2FA)
		api.GET("/site-config", s.getPublicSiteConfig) // 闂傚倸鍊搁崐鐑芥嚄閸洍鈧箓宕煎婵堟嚀椤繄鎹勯搹璇℃Ф婵犵數鍋涘Λ娆撳垂閸洩缍栭柡鍥ュ灪閻撴瑩寮堕崼銉х暫婵＄虎鍣ｉ弻娑欐償閿涘嫅褔鏌＄仦鍓ф创鐎殿喗鎸虫俊鎼佸Ψ閵壯屽晪缂傚倸鍊风粈渚€藝椤栨熬鑰块弶鍫氭櫆椤洟鏌熼悜姗嗘當缂侇偄绉归弻宥堫檨闁告挾鍠栭獮鍐倻閽樺鍘搁梺鍛婁緱閸樺ジ宕戝澶嬧拺闂傚牊绋撴晶鏇熺箾鐏炲倸濡垮?
		// 闂傚倸鍊搁崐鐑芥倿閿曗偓椤啴宕归鍛姺闂佺鍕垫當缂佲偓婢跺备鍋撻獮鍨姎妞わ富鍨跺浼村Ψ閿斿墽顔曢梺鐟邦嚟閸婃垵顫濈捄鍝勫殤闂佸搫绋侀崢浠嬫偂韫囨挴鏀介柣鎰皺娴犮垽鏌涢弮鈧畝鎼佸蓟閿熺姴纾兼俊銈傚亾濞存粓绠栧濠氬磼濞嗘埈妲梺纭咁嚋缁辨洟宕氶幒鎴犳殕闁告洦鍓欓崜褰掓⒑缁嬭法鐏遍柛瀣仱瀹曟垵螣閼姐倗顔曢梺绯曞墲閿氶柣蹇ョ畵閹?(闂傚倸鍊搁崐鐑芥嚄閸洍鈧箓宕煎婵堟嚀椤繄鎹勯搹璇℃Ф婵犵數鍋涘Λ娆撳垂閸洩缍栭柡鍥ュ灪閻撴瑩寮堕崼銉х暫婵＄虎鍣ｉ弻锟犲椽娴ｇ鈷嬮梺璇″枟閿曘垽骞冨▎鎾冲瀭妞ゆ劧绲胯ぐ褔姊绘担鑺ャ€冮梻鍕Ч瀹曟繈骞嬪┑鎰稁婵犵數濮甸懝鍓х矆鐎ｎ偁浜滈柟鍝勬娴滅偓绻濆▓鍨灈闁绘绻掑Σ?
		api.POST("/register", RateLimitMiddleware(s.loginLimiter), s.register)
		api.POST("/verify-email", s.verifyEmail)
		api.POST("/forgot-password", RateLimitMiddleware(s.loginLimiter), s.forgotPassword)
		api.POST("/reset-password", s.resetPassword)
		api.GET("/registration-status", s.getRegistrationStatus)

		// 闂傚倸鍊搁崐鎼佸磹閹间礁纾圭紒瀣紩濞差亝鍋愰悹鍥皺閿涙盯姊洪悷鏉库挃缂侇噮鍨跺畷鎴︽晸閻樺磭鍘搁梺鎼炲劘閸斿酣銆傞懠顒傜＜濞达絽鎼。濂告煏閸パ冾伃妤犵偞锕㈤幃鐑藉级閹稿骸鍔掗梻鍌欑缂嶅﹪藟閹惧瓨鍙忛柣銏㈩焾缁狀垶鏌涘☉妯兼憼闁稿﹦绮穱濠囶敍濮橆剚鍊┑鐐茬墛閹倸顫忕紒妯诲缂佹稑顑嗙紞鍫熺箾鐎涙鐭嗙紒顔界懃閻ｇ兘寮撮悢鍝ョФ濡炪倖鍔楁慨鍐残?
		auth := api.Group("")
		auth.Use(s.authMiddleware())
		auth.Use(APIRateLimitMiddleware(s.globalAPILimiter)) // 闂傚倸鍊搁崐鐑芥嚄閸洍鈧箓宕奸姀鈥冲簥闂佸壊鍋侀崕杈╃矆婢跺备鍋撻崗澶婁壕闂佸憡娲﹂崜娆撳礈?API 闂傚倸鍊搁崐鎼佸磹閹间礁纾归柛婵勫劗閸嬫挸顫濋悡搴＄睄閻庤娲樼换鍫濐嚕娴犲鏁冮柕鍫濇川閸?
		auth.Use(s.viewerWriteBlockMiddleware())
		{
			// 缂傚倸鍊搁崐鎼佸磹閹间礁纾归柣鎴ｅГ閸ゅ嫰鏌涢锝嗙缂佹劖顨婇弻锟犲炊閵夈儳鍔撮梺?
			auth.GET("/stats", s.getStats)

			// 闂傚倸鍊搁崐鐑芥嚄閸洍鈧箓宕奸姀鈥冲簥闂佸壊鍋侀崕杈╃矆婢跺备鍋撻崗澶婁壕闂佸憡娲﹂崜娆撳礈閸愬樊娓婚柕鍫濇缁楁帡鎮楀鐓庡缂侇喛宕甸幉鎾礋椤撶姷妲囧┑鐘垫暩婵挳宕愯ぐ鎺戦棷鐟滅増甯楅悡?
			auth.GET("/search", s.globalSearch)

			// 婵犵數濮烽弫鎼佸磻閻愬樊鐒芥繛鍡樻尭鐟欙箓鎮楅敐搴′簽闁崇懓绉电换娑橆啅椤旇崵鍑归梺绋块閿曨亪寮婚敐澶嬫櫜闁告侗鍨虫导鍥ㄧ箾閹寸偞灏紒澶婄秺瀵濡搁妷銏☆潔濠碘槅鍨拃锔界妤ｅ啯鈷?
			auth.GET("/sessions", s.getSessions)
			auth.DELETE("/sessions/:id", s.deleteSession)
			auth.DELETE("/sessions/others", s.deleteOtherSessions)

			// 闂傚倸鍊搁崐鐑芥嚄閸洖鍌ㄧ憸鏃堢嵁閺嶎収鏁冮柨鏇楀亾缁惧墽鎳撻埞鎴︽偐鐎圭姴顥濈紒鐐劤椤兘寮婚妸銉㈡斀闁糕剝锚缁愭稒绻涢幋鐐村碍缂佸缍婂濠氬Ω閵夈垺顫嶅┑鈽嗗灟鐠€锔界妤ｅ啯鈷?(闂傚倸鍊搁崐椋庣矆娓氣偓楠炲鏁撻悩鑼槷闂佸搫绋侀崢浠嬪磻閿熺姵鐓ラ柡鍥╁仜閳ь剙鎽滅划鏄忋亹閹烘挴鎷洪梺鍓茬厛閸ｎ噣宕曢幇顑芥斀妞ゆ牗姘ㄩ幗鐘电磼缂佹绠栫紒缁樼箞瀹曟帒顫濋梻瀛樺亝濠电姷鏁搁崑娑㈠触鐎ｎ喖鍨傞柛褎顨呴拑鐔哥箾閹存瑥鐏╃紒顐㈢Ч閺屾稓浠﹂幆褏鍔伴梺鍝勵儏閹冲酣鈥旈崘顔嘉ч柛鎰╁妷閸嬫捇寮撮姀鐘碉紮闂佽崵鍠栭崑濠囧吹濡ゅ懏鐓曢柡鍥ュ妼閻忕娀鏌涢幇銊ヤ壕濠碉紕鍋戦崐鏍箰妤ｅ啫闂柕澶嗘櫅閻掑灚銇勯幒宥堝厡闁瑰啿瀚槐?
			auth.GET("/nodes", s.listNodes)
			auth.GET("/nodes/paginated", s.listNodesPaginated)
			auth.POST("/nodes", APIRateLimitMiddleware(s.writeAPILimiter), s.createNode)
			auth.GET("/nodes/:id", s.getNode)
			auth.PUT("/nodes/:id", APIRateLimitMiddleware(s.writeAPILimiter), s.updateNode)
			auth.DELETE("/nodes/:id", APIRateLimitMiddleware(s.writeAPILimiter), s.deleteNode)
			auth.POST("/nodes/:id/apply", APIRateLimitMiddleware(s.writeAPILimiter), s.applyNodeConfig)
			auth.POST("/nodes/:id/clone", APIRateLimitMiddleware(s.writeAPILimiter), s.cloneNode)
			auth.POST("/nodes/:id/sync", APIRateLimitMiddleware(s.writeAPILimiter), s.syncNodeConfig)
			auth.GET("/nodes/:id/gost-config", s.getNodeGostConfig)
			auth.GET("/nodes/:id/proxy-uri", s.getNodeProxyURI)
			auth.GET("/nodes/:id/install-script", s.getNodeInstallScript)
			auth.GET("/nodes/:id/ping", s.pingNode)
			auth.GET("/nodes/ping", s.pingAllNodes)
			auth.GET("/nodes/:id/health-logs", s.getNodeHealthLogs)
			auth.GET("/health-summary", s.getHealthSummary)

			// 闂傚倸鍊搁崐鐑芥嚄閸洖鍌ㄧ憸鏃堢嵁閺嶎収鏁冮柨鏇楀亾缁惧墽鎳撻埞鎴︽偐鐎圭姴顥濈紒鐐劤椤兘寮婚妸銉㈡斀闁糕檧鏅滄晥闂備胶绮幐鎾磻閹剧粯鐓熼幖杈剧磿閻ｎ參鏌涙惔鈥宠埞閻撱倝鏌曟繛鐐珔闁告垹濞€閺屾稑鐣濋埀顒勫磻閻愮儤鍋傞柣鏃堟櫜缁诲棝鏌曢崼婵嗏偓鍛婄妤ｅ啯鈷戠紓浣癸供濞堟棃鏌ㄩ弴妯衡偓婵嬪极閸愵喖顫呴柍銉﹀墯閸ゃ倝姊洪崫鍕垫Ч闁搞劎鏁诲畷鎴濐吋婢跺鎷虹紓渚囧灡濞叉牗鏅堕崣澶堜簻闁靛鍎查崵鍥煙?
			auth.GET("/nodes/:id/config-versions", s.getConfigVersions)
			auth.POST("/nodes/:id/config-versions", s.createConfigVersion)
			auth.GET("/config-versions/:versionId", s.getConfigVersion)
			auth.POST("/config-versions/:versionId/restore", s.restoreConfigVersion)
			auth.DELETE("/config-versions/:versionId", s.deleteConfigVersion)

			// 闂傚倸鍊搁崐鐑芥嚄閸洖鍌ㄧ憸鏃堢嵁閺嶎収鏁冮柨鏇楀亾缁惧墽鎳撻埞鎴︽偐鐎圭姴顥濈紒鐐劤椤兘寮婚妸銉㈡斀闁糕檧鏅滅瑧缂傚倷鑳舵慨浼村磿閹惰棄绠熼柟闂寸劍閸嬪鏌涢锝囩畼闁荤喆鍔岄埞鎴︽倷閸欏娅￠梺绋匡攻缁诲牆顕ｆ繝姘亜闁绘挸楠搁悗顓烆渻閵堝棙鈷愰悗姘叄閺佸秹宕熼鐙呯闯?
			auth.POST("/nodes/batch-enable", s.batchEnableNodes)
			auth.POST("/nodes/batch-disable", s.batchDisableNodes)
			auth.POST("/nodes/batch-delete", s.batchDeleteNodes)
			auth.POST("/nodes/batch-sync", s.batchSyncNodes)

			auth.GET("/clients", s.listClients)
			auth.GET("/clients/paginated", s.listClientsPaginated)
			auth.POST("/clients", s.createClient)
			auth.GET("/clients/:id", s.getClient)
			auth.PUT("/clients/:id", s.updateClient)
			auth.DELETE("/clients/:id", s.deleteClient)
			auth.GET("/clients/:id/install-script", s.getClientInstallScript)
			auth.GET("/clients/:id/gost-config", s.getClientGostConfig)
			auth.GET("/clients/:id/proxy-uri", s.getClientProxyURI)
			auth.POST("/clients/:id/clone", s.cloneClient)

			auth.POST("/clients/batch-enable", s.batchEnableClients)
			auth.POST("/clients/batch-disable", s.batchDisableClients)
			auth.POST("/clients/batch-delete", s.batchDeleteClients)
			auth.POST("/clients/batch-sync", s.batchSyncClients)

			// 闂傚倸鍊搁崐鐑芥倿閿曗偓椤啴宕归鍛姺闂佺鍕垫當缂佲偓婢跺备鍋撻獮鍨姎妞わ富鍨跺浼村Ψ閿斿墽顔曢梺鐟邦嚟閸嬬偤鎯冮幋婵冩闁规儳纾晥闂佸搫琚崝宀勫煡婢跺á鐔哥瑹椤栨瑧妫梻?
			auth.GET("/users", s.listUsers)
			auth.POST("/users", s.createUser)
			auth.GET("/users/:id", s.getUser)
			auth.PUT("/users/:id", s.updateUser)
			auth.DELETE("/users/:id", s.deleteUser)
			auth.POST("/change-password", s.changePassword)

			// 婵犵數濮烽弫鎼佸磻閻愬搫鍨傞柛顐ｆ礀缁犲綊鏌嶉崫鍕偓濠氥€呴崣澶岀闁糕剝顨夐鐔烘喐鎼粹槅鍤楅柛鏇ㄥ幒濞岊亪鏌熼鍡楀缁扁晠姊婚崒娆戝妽閻庣瑳鍏炬稒鎷呴懖婵囩洴瀹曠喖顢涘☉妯圭暗闂備礁鎼ú銏ゅ垂濞差亜纾婚柨婵嗘偪瑜版帗鏅查柛銉ュ閸旀悂姊洪幎鑺ユ暠婵☆偄鍟村?
			auth.GET("/profile", s.getProfile)
			auth.PUT("/profile", s.updateProfile)

			auth.POST("/profile/2fa/enable", s.enable2FA)
			auth.POST("/profile/2fa/verify", s.verify2FA)
			auth.POST("/profile/2fa/disable", s.disable2FA)

			// 濠电姷鏁告慨鐑藉极閹间礁纾婚柣鏃傚劋瀹曞弶绻濋棃娑氬妞ゆ劘濮ら幈銊ノ熼幐搴ｃ€愰梻鍌氬亞閸ㄨ京鎹㈠☉姗嗗晠妞ゆ棁宕甸惄搴ㄦ⒑缁嬭儻顫﹂柛鏃€鍨垮璇测槈閵忕姷鍔撮梺鍛婂姉閸嬫捇鎮鹃崼鏇熲拺?
			auth.GET("/traffic-history", s.getTrafficHistory)

			// 闂傚倸鍊搁崐鎼佸磹妞嬪孩顐介柨鐔哄Т绾惧鏌涘☉鍗炲季婵炲皷鏅犻弻鏇熺箾閻愵剚鐝曢梺绋款儏閸婂潡寮诲澶娢ㄧ憸宥嗙濞差亝鐓冪憸婊堝礈濮樿鲸鏆滈柨鐔哄Т缁犳牠鏌嶉崫鍕殲濠殿垰銈搁弻娑㈠箛椤掍讲鏋欏┑鈽嗗亝缁海妲愰幘璇茬＜婵炲棙甯掗崢鈩冪節濞堝灝鐏犳い鏇ㄥ幖鍗遍柟鐗堟緲閸楁娊鏌曡箛鏇炐ラ柣?
			auth.GET("/notify-channels", s.listNotifyChannels)
			auth.POST("/notify-channels", s.createNotifyChannel)
			auth.GET("/notify-channels/:id", s.getNotifyChannel)
			auth.PUT("/notify-channels/:id", s.updateNotifyChannel)
			auth.DELETE("/notify-channels/:id", s.deleteNotifyChannel)
			auth.POST("/notify-channels/:id/test", s.testNotifyChannel)

			// 闂傚倸鍊搁崐椋庣矆娓氣偓楠炲鏁嶉崟顓犵厯闂佺鎻梽鍕疾濠靛鐓忓┑鐐靛亾濞呮捇鏌℃担鍛婎棦闁哄矉缍佸顕€宕掑顒€顬嗛梻浣告憸婵潙螞濠靛钃熸繛鎴欏灩閸楁娊鏌曟繛鍨姍缂併劌顭峰铏规嫚閼碱剛顔婇梺鍝ュУ閹告儳危閹版澘绠虫俊銈勭娴滃ジ姊洪崨濠佺繁闁搞劍濞婇弫宥夋偄閸忓皷鎷?
			auth.GET("/alert-rules", s.listAlertRules)
			auth.POST("/alert-rules", s.createAlertRule)
			auth.GET("/alert-rules/:id", s.getAlertRule)
			auth.PUT("/alert-rules/:id", s.updateAlertRule)
			auth.DELETE("/alert-rules/:id", s.deleteAlertRule)

			// 闂傚倸鍊搁崐椋庣矆娓氣偓楠炲鏁嶉崟顓犵厯闂佺鎻梽鍕疾濠靛鐓忓┑鐐靛亾濞呮捇鏌℃担鍛婎棦闁哄矉缍佸顕€宕掑顑跨帛缂傚倷绶￠崑鍕矓瑜版帒钃熼柡鍥╁枎缁剁偤鏌涢锝囩畵濠殿喓鍨荤槐?
			auth.GET("/alert-logs", s.getAlertLogs)

			// 闂傚倸鍊搁崐鐑芥嚄閸洖绠犻柟鍓х帛閸婅埖绻濋棃娑氬ⅱ闁活厽鎸鹃埀顒冾潐濞叉牕煤閵娧呯焼濠电姴娲﹂悡娑㈡煕閹扳晛濡垮褎娲滅槐鎺楊敃閵忊懣褔鏌＄仦鐐鐎垫澘瀚板畷鐓庘攽閸℃ぅ鎴犵磽?
			auth.GET("/operation-logs", s.getOperationLogs)

			// 闂傚倸鍊搁崐宄懊归崶褜娴栭柕濞炬櫆閸ゅ嫰鏌ょ粙璺ㄤ粵婵炲懐濮垫穱濠囧Χ閸屾矮澹曢梻浣风串缁蹭粙鎮樺璺虹闁告侗鍨遍崑姗€鏌嶉妷銉ュ笭濠㈣娲熷鍝勑ч崶褏浼堝┑鐐板尃閸涱喗娈伴梺鍓插亝濞叉﹢鎮?闂傚倸鍊峰ù鍥敋瑜嶉湁闁绘垼妫勯弸渚€鏌熼梻瀵割槮闁稿被鍔庨幉鎼佸棘鐠恒劍娈?
			auth.GET("/export", s.exportData)
			auth.POST("/import", s.importData)

			// 闂傚倸鍊搁崐宄懊归崶褜娴栭柕濞炬櫆閸ゅ嫰鏌ょ粙璺ㄤ粵婵炲懐濮垫穱濠囧Χ閸屾矮澹曢梻浣风串缁蹭粙鎮樺璺虹闁告侗鍨遍崰鍡涙煕閺囥劌浜滃┑鈩冨▕濮婄粯鎷呴崨濠呯闂佺顑嗛幑鍥х暦濠婂喚娼╂い鎴旀櫔缂嶄礁螞閸愵煁?闂傚倸鍊搁崐宄懊归崶顒夋晪闁哄稁鍘奸崒銊ф喐閻楀牆绗掗柛銊ュ€婚幉鎼佹偋閸繂鎯?
			auth.GET("/backup", s.backupDatabase)
			auth.POST("/restore", s.restoreDatabase)

			// 缂傚倸鍊搁崐鎼佸磹閻戣姤鍊块柨鏇楀亾閾荤偤鐓崶銊р槈闁搞劌鍊块弻鐔风暋閹峰矈娼舵繛瀛樼矎椤鎹㈠☉銏犵闁绘劕鐏氶崳顓熺節濞堝灝鏋熺紒顔肩Ч婵＄敻宕熼姘鳖唺闂佺硶鍓濋妵鐐寸珶閺囥垺鐓?
			auth.GET("/port-forwards", s.listPortForwards)
			auth.POST("/port-forwards", s.createPortForward)
			auth.GET("/port-forwards/:id", s.getPortForward)
			auth.PUT("/port-forwards/:id", s.updatePortForward)
			auth.DELETE("/port-forwards/:id", s.deletePortForward)
			auth.POST("/port-forwards/:id/clone", s.clonePortForward)

			// 闂傚倸鍊搁崐鐑芥嚄閸洖鍌ㄧ憸鏃堢嵁閺嶎収鏁冮柨鏇楀亾缁惧墽鎳撻埞鎴︽偐鐎圭姴顥濈紒鐐劤椤兘寮婚妸銉㈡斀闁糕剝锚缁愭稓绱?(闂傚倸鍊峰ù鍥х暦閸偅鍙忛柣銏㈩焾缁€澶嬩繆閵堝懏鍣圭紒鎰殜閺岋繝宕堕埡鈧槐宕囨喐閻楀牆绗掗柣鎰攻閵囧嫰骞掗崱妞惧闂傚倷娴囬敓銉╁磹濠靛钃熸繛鎴欏灩鍞銈嗘瀹曠敻寮冲Δ鍛拺闂侇偆鍋涢懟顖炲储閹绢喗鐓?
			auth.GET("/node-groups", s.listNodeGroups)
			auth.POST("/node-groups", s.createNodeGroup)
			auth.GET("/node-groups/:id", s.getNodeGroup)
			auth.PUT("/node-groups/:id", s.updateNodeGroup)
			auth.DELETE("/node-groups/:id", s.deleteNodeGroup)
			auth.GET("/node-groups/:id/members", s.listNodeGroupMembers)
			auth.POST("/node-groups/:id/members", s.addNodeGroupMember)
			auth.DELETE("/node-groups/:id/members/:memberId", s.removeNodeGroupMember)
			auth.GET("/node-groups/:id/config", s.getNodeGroupConfig)
			auth.POST("/node-groups/:id/clone", s.cloneNodeGroup)

			// 婵犵數濮烽弫鎼佸磻濞戙埄鏁嬫い鎾跺枎閸ㄦ棃鎮楅悽娈跨劸缂佽翰鍊濋弻娑滎槼妞ゃ劌鎳愮划濠氬冀瑜夐弨浠嬫煟濡搫绾у璺哄閺?闂傚倸鍊搁崐鎼佸磹閹间礁纾归柟闂寸缁犵喖鏌ｉ幇顒佹儓闁绘劕锕弻銊╁即濡も偓娴滄儳顪冮妶鍐ㄥ姕鐎光偓閹间礁钃熼柍鈺佸暙缁剁偤鎮橀悙鑸殿棄濠㈢懓顦靛娲川婵犲啫闉嶉梺鑽ゅ枂閸旀垶淇?
			auth.GET("/proxy-chains", s.listProxyChains)
			auth.POST("/proxy-chains", s.createProxyChain)
			auth.GET("/proxy-chains/:id", s.getProxyChain)
			auth.PUT("/proxy-chains/:id", s.updateProxyChain)
			auth.DELETE("/proxy-chains/:id", s.deleteProxyChain)
			auth.GET("/proxy-chains/:id/hops", s.listProxyChainHops)
			auth.POST("/proxy-chains/:id/hops", s.addProxyChainHop)
			auth.PUT("/proxy-chains/:id/hops/:hopId", s.updateProxyChainHop)
			auth.DELETE("/proxy-chains/:id/hops/:hopId", s.removeProxyChainHop)
			auth.GET("/proxy-chains/:id/config", s.getProxyChainConfig)
			auth.POST("/proxy-chains/:id/clone", s.cloneProxyChain)

			// 闂傚倸鍊搁崐鎼佸磹閹间礁纾归柟闂寸缁犵喖鏌ｉ幇顒佹儓闁绘劕锕弻銊╁即濡も偓娴滄儳顪冮妶鍐ㄥ姕鐎光偓閹间礁钃熼柍鈺佸暙缁剁偤鎮橀悙鑸殿棄濠㈢懓顦靛娲川婵犲啫闉嶉梺鑽ゅ枂閸旀垶淇?(闂傚倸鍊搁崐鐑芥嚄閸洍鈧箓宕奸姀鈥冲簥闂佸壊鍋侀崕杈╁閸ф绾ч柛顐亜娴滄牕霉?闂傚倸鍊搁崐椋庣矆娓氣偓楠炲鏁撻悩鎻掔€梺姹囧灮椤牏绮荤憴鍕╀簻闁规壋鏅涢崝妯好瑰鍡╂綈缂佺粯鐩獮瀣倻閸℃瑥濮遍梻浣虹帛濮婂宕㈣閹ê鐣烽崶锝呬壕閻熸瑥瀚粈鈧┑鐐茬湴閸旀垿濡?
			auth.GET("/tunnels", s.listTunnels)
			auth.POST("/tunnels", s.createTunnel)
			auth.GET("/tunnels/:id", s.getTunnel)
			auth.PUT("/tunnels/:id", s.updateTunnel)
			auth.DELETE("/tunnels/:id", s.deleteTunnel)
			auth.POST("/tunnels/:id/sync", s.syncTunnel)
			auth.GET("/tunnels/:id/entry-config", s.getTunnelEntryConfig)
			auth.GET("/tunnels/:id/exit-config", s.getTunnelExitConfig)
			auth.POST("/tunnels/:id/clone", s.cloneTunnel)

			auth.GET("/templates", s.listTemplates)
			auth.GET("/templates/categories", s.getTemplateCategories)
			auth.GET("/templates/:id", s.getTemplate)

			auth.GET("/client-templates", s.listClientTemplates)
			auth.GET("/client-templates/categories", s.getClientTemplateCategories)
			auth.GET("/client-templates/:id", s.getClientTemplate)

			// 缂傚倸鍊搁崐鎼佸磹閹间礁纾归柟闂寸閻ゎ喗銇勯弽顐粶闁搞劌鍊婚幉鎼佹偋閸繄鐟查梺绋款儑閸犳劙濡甸崟顖氬唨妞ゆ劦婢€濞岊亪姊虹紒妯诲皑闁稿鎹囬弻锝嗘償閿涘嫮鏆涢梺绋块瀹曨剛鍙呴梺闈浥堥弲娑㈠礄?(婵犵數濮烽弫鎼佸磻濞戙埄鏁嬫い鎾跺枑閸欏繘鏌熺紒銏犳灈缂佺姷濞€閺岀喐瀵奸弶鎴滃枈闂佽桨绀佸ú顓㈠蓟閻斿吋鈷掗悗鐢殿焾婵′粙姊虹粙娆惧剳濠电偐鍋撻梺鍝勬湰閻╊垶宕洪崟顖氱闁绘劦鍓涢弳锕傛⒒?
			auth.GET("/site-configs", s.getSiteConfigs)
			auth.PUT("/site-configs", s.updateSiteConfigs)

			// 闂傚倸鍊搁崐鐑芥嚄閸洖鍌ㄧ憸鏃堢嵁閺嶎収鏁冮柨鏇楀亾缁惧墽鎳撻埞鎴︽偐鐎圭姴顥濈紒鐐劤椤兘寮婚妸銉㈡斀闁糕檧鏅滅瑧闂備礁鎲￠崝蹇涘磻閹剧粯鈷掑ù锝堫潐閸嬬娀鏌涙惔顔兼珝鐎殿喗鐓￠崺锟犲川椤撶媭妲锋俊鐐€栭弻銊︽櫠閻ｅ苯顥氬┑鍌氭啞閻撶喐绻涢崱妤冪闁革絿鏅惀顏堝箚瑜庨崑銉╂煛?
			auth.GET("/tags", s.listTags)
			auth.GET("/tags/:id", s.getTag)
			auth.POST("/tags", s.createTag)
			auth.PUT("/tags/:id", s.updateTag)
			auth.DELETE("/tags/:id", s.deleteTag)
			auth.GET("/tags/:id/nodes", s.getNodesByTag)

			auth.GET("/nodes/:id/tags", s.getNodeTags)
			auth.POST("/nodes/:id/tags", s.addNodeTag)
			auth.PUT("/nodes/:id/tags", s.setNodeTags)
			auth.DELETE("/nodes/:id/tags/:tagId", s.removeNodeTag)

			auth.POST("/users/:id/verify-email", s.adminVerifyUserEmail)
			auth.POST("/users/:id/resend-verification", s.resendVerificationEmail)
			auth.POST("/users/:id/reset-quota", s.resetUserQuota)
			auth.POST("/users/:id/assign-plan", s.assignUserPlan)
			auth.POST("/users/:id/remove-plan", s.removeUserPlan)
			auth.POST("/users/:id/renew-plan", s.renewUserPlan)

			// 婵犵數濮烽弫鍛婄箾閳ь剚绻涙担鍐叉搐绾惧湱鎲搁悧鍫濈闁逞屽墮閸燁垳鎹㈠┑瀣倞濞达絽鎲￠崰姗€鏌熼鑽ょ煓闁诡喕绮欏畷妤呭传閵壯勫枓闂傚倸鍊烽悞锔锯偓绗涘懐鐭欓柟瀵稿Л閸嬫挸顫濋悡搴＄睄閻?
			auth.GET("/plans", s.listPlans)
			auth.GET("/plans/:id", s.getPlan)
			auth.POST("/plans", s.createPlan)
			auth.PUT("/plans/:id", s.updatePlan)
			auth.DELETE("/plans/:id", s.deletePlan)
			auth.GET("/plans/:id/resources", s.getPlanResources)
			auth.PUT("/plans/:id/resources", s.setPlanResources)

			// Bypass 闂傚倸鍊搁崐椋庣矆娓氣偓楠炲鏁嶉崟顒佹闂佸湱鍎ら崵锕€鈽夊Ο閿嬬€婚梺鐟邦嚟婵兘鎮炴ィ鍐┾拺闁圭瀛╅埛鎺楁煕鐎ｎ偅宕岀€规洑鍗冲畷鍗炩槈濞嗘垵骞堥梻浣告惈濞层垽宕濈仦鍓ь洸婵犲﹤鐗婇悡?
			auth.GET("/bypasses", s.listBypasses)
			auth.GET("/bypasses/:id", s.getBypass)
			auth.POST("/bypasses", s.createBypass)
			auth.PUT("/bypasses/:id", s.updateBypass)
			auth.DELETE("/bypasses/:id", s.deleteBypass)
			auth.POST("/bypasses/:id/clone", s.cloneBypass)

			// Admission 闂傚倸鍊搁崐椋庣矆娓氣偓楠炲鏁撻悩鎻掔€梻鍌氱墛閸忔艾鈽夊Ο婊勬瀹曘劑顢橀悩闈涘箑闂傚倷鑳剁涵鍫曞礈濠靛牏鐭欓柟瀵稿Х娑撳秶鈧娲栧ú锕傚窗閹邦喒鍋撻獮鍨姎妞わ富鍨跺?
			auth.GET("/admissions", s.listAdmissions)
			auth.GET("/admissions/:id", s.getAdmission)
			auth.POST("/admissions", s.createAdmission)
			auth.PUT("/admissions/:id", s.updateAdmission)
			auth.DELETE("/admissions/:id", s.deleteAdmission)
			auth.POST("/admissions/:id/clone", s.cloneAdmission)

			// HostMapping 婵犵數濮烽弫鎼佸磻閻愬搫鍨傞柛顐ｆ礀缁犱即鏌涢幘妤€鍟悘濠傤渻閵堝棛澧柛鈺佸暣瀵劑鏁冮崒娑氬帗閻熸粍绮撳畷婊堟偄閻撳氦鎽曢梺璺ㄥ枔婵绮堢€ｎ偁浜滈柟鎹愭硾閳ь剦鍋婃慨鈧柕鍫濇閸?
			auth.GET("/host-mappings", s.listHostMappings)
			auth.GET("/host-mappings/:id", s.getHostMapping)
			auth.POST("/host-mappings", s.createHostMapping)
			auth.PUT("/host-mappings/:id", s.updateHostMapping)
			auth.DELETE("/host-mappings/:id", s.deleteHostMapping)
			auth.POST("/host-mappings/:id/clone", s.cloneHostMapping)

			// Ingress 闂傚倸鍊搁崐椋庣矆娓氣偓楠炲鏁撻悩鍐蹭画闂侀潧锛忛崨顖滃帬闂備礁婀遍搹搴ㄥ窗閺嶎偆涓嶉柡宥冨妿缁♀偓婵犵數濮撮崐鎼侇敂椤愶絻浜滈柍鍝勫暙閸樻挳鏌＄仦璇插闁宠鍨垮畷鍗炍熸笟顖浶熼梻?
			auth.GET("/ingresses", s.listIngresses)
			auth.GET("/ingresses/:id", s.getIngress)
			auth.POST("/ingresses", s.createIngress)
			auth.PUT("/ingresses/:id", s.updateIngress)
			auth.DELETE("/ingresses/:id", s.deleteIngress)
			auth.POST("/ingresses/:id/clone", s.cloneIngress)

			// Recorder 濠电姷鏁告慨鐑藉极閹间礁纾婚柣鏃傚劋瀹曞弶绻濋棃娑氬妞ゆ劘濮ら幈銊ノ熼幐搴ｃ€愰梻鍌氬亞閸ㄥ爼寮昏缁犳稑顫濋崗鑲╃Х濠电姵顔栭崹浼村Χ閹间礁钃熼柍鈺佸暙缁剁偤骞栧ǎ顒€鐏柕鍡楋功缁?
			auth.GET("/recorders", s.listRecorders)
			auth.GET("/recorders/:id", s.getRecorder)
			auth.POST("/recorders", s.createRecorder)
			auth.PUT("/recorders/:id", s.updateRecorder)
			auth.DELETE("/recorders/:id", s.deleteRecorder)
			auth.POST("/recorders/:id/clone", s.cloneRecorder)

			// Router 闂傚倸鍊峰ù鍥х暦閸偅鍙忕€规洖娲︽刊浼存煥閺囩偛鈧悂宕归崒鐐寸厵闁诡垳澧楅ˉ澶愭煛鐎ｂ晝绐旂€殿喖鐖煎畷鐓庮潩椤撶喓褰囧┑鐐村灦閹稿摜绮旇ぐ鎺戣摕闁靛鍎Σ鍫熶繆椤栨瑨顒熷ù鐘荤畺濮?
			auth.GET("/routers", s.listRouters)
			auth.GET("/routers/:id", s.getRouter)
			auth.POST("/routers", s.createRouter)
			auth.PUT("/routers/:id", s.updateRouter)
			auth.DELETE("/routers/:id", s.deleteRouter)
			auth.POST("/routers/:id/clone", s.cloneRouter)

			// SD 闂傚倸鍊搁崐椋庣矆娓氣偓楠炴牠顢曢敂钘変罕闂佺硶鍓濋悷褔鎯岄幘缁樺€垫繛鎴烆伆閹达箑鐭楅煫鍥ㄧ⊕閻撶喖鏌￠崘銊モ偓鍝ユ暜閸洘鐓熸い鎾跺櫏濞堟粓鏌″畝鈧崰鎾诲焵椤掑倹鏆╂い顓炵墕閻☆厾绱?
			auth.GET("/sds", s.listSDs)
			auth.GET("/sds/:id", s.getSD)
			auth.POST("/sds", s.createSD)
			auth.PUT("/sds/:id", s.updateSD)
			auth.DELETE("/sds/:id", s.deleteSD)
			auth.POST("/sds/:id/clone", s.cloneSD)
		}
	}

	// Agent 闂傚倸鍊搁崐宄懊归崶顒婄稏濠㈣泛顑囬々鎻捗归悩宸剰缁炬儳娼″鍫曞醇椤愵澀绨存繛?(婵犵數濮烽弫鎼佸磻閻樿绠垫い蹇撴缁€濠囨煃瑜滈崜姘辨崲濞戞瑥绶為悗锝庡亞椤︿即鎮?Token 闂傚倸鍊峰ù鍥х暦閸偅鍙忛柟鎯板Г閳锋梻鈧箍鍎遍ˇ顖滃鐟欏嫮绠鹃柟瀛樼懃閻忣亪鏌涙惔鈽呭伐妞ゎ厼娼￠幊婊堟濞戞﹩娼旈梻浣告惈閹峰宕滈悢鐓庣畺婵せ鍋撻柟顔界懇瀹曞綊顢曢敐鍥ф锭闂傚倷娴囧銊╂⒔瀹ュ绀夐柟瀛樼箥濞兼牕鈹戦悩瀹犲缂佲偓鐎ｎ偁浜滈柟鍝勬娴滅偓绻濆▓鍨灈闁绘绻掑Σ?
	agent := s.router.Group("/agent")
	agent.Use(APIRateLimitMiddleware(s.agentLimiter))
	{
		agent.POST("/register", s.agentRegister)
		agent.POST("/heartbeat", s.agentHeartbeat)
		agent.GET("/config/:token", s.agentGetConfig)
		agent.GET("/version", s.agentGetVersion)
		agent.GET("/check-update", s.agentCheckUpdate)
		agent.GET("/download/:os/:arch", s.agentDownload)
		// 闂傚倸鍊峰ù鍥敋瑜庨〃銉╁传閵壯傜瑝閻庡箍鍎遍ˇ顖炲垂閸屾稓绠剧€瑰壊鍠曠花濠氭煛閸曗晛鍔滅紒缁樼洴楠炲鎮欑€靛憡顓婚梻浣告啞椤ㄥ棛鍠婂澶娢﹂柛鏇ㄥ灠閸愨偓闂侀潧顭俊鍥р枔閵堝棛绡€缁剧増菤閸嬫捇鎼归銏＄亷闂?(闂傚倸鍊搁崐鎼佸磹妞嬪孩顐介柨鐔哄Т绾惧鏌涘☉鍗炴灓闁崇懓绉归弻褑绠涘鍏肩秷閻?token 闂傚倸鍊峰ù鍥х暦閸偅鍙忛柟鎯板Г閳锋梻鈧箍鍎遍ˇ顖滃鐟欏嫮绠鹃柟瀛樼懃閻忣亪鏌?
		agent.POST("/client-heartbeat/:token", s.clientHeartbeat)
	}

	// WebSocket 闂傚倸鍊搁崐宄懊归崶顒婄稏濠㈣泛顑囬々鎻捗归悩宸剰缁炬儳娼″鍫曞醇椤愵澀绨存繛?(闂傚倸鍊搁崐鎼佸磹閹间礁纾圭紒瀣紩濞差亝鍋愰悹鍥皺閿涙盯姊洪悷鏉库挃缂侇噮鍨跺畷鎴︽晸閻樺磭鍘搁梺鎼炲劘閸斿酣銆傞懠顒傜＜濞达絽鎼。濂告煏閸パ冾伃妤犵偞锕㈤幃鐑藉级閹稿骸鍔掗梻?
	s.router.GET("/ws", s.wsAuthMiddleware(), s.handleWebSocket)

	// 闂傚倸鍊峰ù鍥敋瑜嶉湁闁绘垼妫勭粻鐘绘煙閹冩闁搞儺鍓欑粻顕€鏌涢幘宕囦虎妞わ附澹嗛幑銏犫攽鐎ｎ亞鍊為梺闈涱焾閸庢煡鎮剧紒妯肩瘈婵炲牆鐏濋弸鐔兼煥閺囨娅婄€规洏鍨虹粋鎺斺偓锝庝簽椤斿棝姊绘笟鍥у缂佸鏁婚幃鈥斥枎閹惧鍘靛銈嗘尵閸犲海绮幒妤佺厱闁哄啠鍋撴慨妯稿姂楠?(闂傚倸鍊搁崐鐑芥嚄閸洍鈧箓宕煎婵堟嚀椤繄鎹勯搹璇℃Ф婵犵數鍋涘Λ娆撳垂閸洩缍?
	scripts := s.router.Group("/scripts")
	{
		scripts.GET("/install-node.sh", s.serveInstallScript("install-node.sh"))
		scripts.GET("/install-client.sh", s.serveInstallScript("install-client.sh"))
		scripts.GET("/install-node.ps1", s.serveInstallScript("install-node.ps1"))
		scripts.GET("/install-client.ps1", s.serveInstallScript("install-client.ps1"))
		// 闂傚倸鍊搁崐椋庣矆娓氣偓楠炲鏁撻悩鑼舵憰闂佹寧绻傞ˇ顖滅矆婢跺备鍋撻獮鍨姎妞わ富鍨堕幃鐐哄垂椤愮姳绨婚梺纭呮彧缁蹭粙宕濋悽鍛婄厱闊洦鎸鹃悞鎼佹煛鐏炵澧查柟宄版噽閹叉挳宕熼銈忕处濠德板€楁慨鐑藉磻濞戙埄鏁勫鑸靛姉瀹撲線鏌涢妷顔煎⒒闁轰礁锕弻锝夋晲閸涱喗鎷卞┑鐐茬墛閹倸顫忛搹瑙勫珰闁告瑥顦板畷鎶芥⒑绾懏鐝柟鐟版穿濡喖姊虹涵鍛【濠㈣鐟﹀鍕偓锝庡墰閻﹀牓姊洪幖鐐插姌闁告柨鑻埢鎾澄旈埀顒勫煘閹达富鏁婇柡鍌樺€撶欢鐢告⒑閸涘⊕鑲╁垝濞嗘挾宓?(闂傚倸鍊搁崐鎼佸磹妞嬪孩顐介柨鐔哄Т绾惧鏌涘☉鍗炴灓闁崇懓绉归弻褑绠涘鍏肩秷閻?token 闂傚倸鍊峰ù鍥х暦閸偅鍙忛柟鎯板Г閳锋梻鈧箍鍎遍ˇ顖滃鐟欏嫮绠鹃柟瀛樼懃閻忣亪鏌?
		scripts.GET("/client/:token", s.serveClientScript)
	}

	// 闂傚倸鍊搁崐鎼佸磹閹间礁纾归柟闂寸劍閸嬪鈹戦悩鍙夊闁稿绻濋弻銊╁即閻愭祴鍋撻幖浣瑰€块柛顭戝亖娴滄粓鏌熸潏鍓хɑ缁绢厼鐖奸弻娑㈠棘鐠恒剱褔鏌″畝瀣？闁逞屽墾缂嶅棙绂嶅鍫濇辈闁绘劗鏁哥壕?(闂傚倸鍊搁崐椋庣矆娓氣偓楠炲鏁撻悩鑼唶闂佸憡绺块崕鎶芥儗閹剧粯鐓曟い鎰剁稻婢跺嫰鏌?
	s.setupStaticFiles()
}

func (s *Server) setupStaticFiles() {
	subFS, err := fs.Sub(staticFS, "dist")
	if err != nil {
		return
	}

	// 闂傚倸鍊搁崐鎼佸磹閹间礁纾归柟闂寸劍閸嬪鈹戦悩鍙夊闁稿绻濋弻銊╁即閻愭祴鍋撻幖浣瑰€块柛顭戝亖娴滄粓鏌熺€电浠滄い鏇熺矌缁辨帗鎷呴悷閭︽缂備浇椴哥敮妤冪箔閻旂厧鐐婄憸宀€鑺辨繝姘拺鐎规洖娲﹂崵鈧梺鍝ュ枑閹告娊鎮伴鈧畷鍫曨敆婢跺娅嶉梻浣虹帛濡線濡撮埀顒勬煕鐎ｎ偅宕屾い銏＄懃闇?- 闂傚倸鍊搁崐椋庣矆娴ｈ櫣绀婂┑鐘插亞閻掔晫鎲搁弮鍫涒偓渚€寮介鐐茬獩濡炪倖妫佸Λ鍕嚕閸ф鈷戦柛鎰级閹牓鏌涙繝鍌涙喐婵炲棎鍨介幃娆撴倻濡厧骞堥梻浣告惈濞层垽宕濆畝鍕祦婵°倕鎳忛悡鏇炩攽閻樻彃顏╁ù鐘崇矒濡焦寰勯幇顓犲幗濠殿喗顨呭Λ妤呭几濞戙垺鐓曢柡鍌濐嚙閳ь剚绻堝璇差吋婢跺á銊╁窗閸岀偛绠ｉ柣妯兼暩閻撴棃姊洪崨濠庢畼闁稿鍔欏銊︾鐎ｎ偆鍘遍梺闈涱槶閸斿秶娑甸幆褉鏀介柍顖涚懃閹虫劗澹曢懖鈺冪＝濞达綁娼ч悘鈺冣偓娈垮櫍缁犳牠寮诲☉銏″亹閻庡湱濮撮ˉ婵嬫⒑?MIME 缂傚倸鍊搁崐鎼佸磹妞嬪孩顐介柨鐔哄Т绾捐顭块懜闈涘Е闁轰礁顑囬幉鎼佸籍閸繄鐣?
	s.router.GET("/assets/*filepath", func(c *gin.Context) {
		fp := c.Param("filepath")
		path := "assets" + fp
		data, err := fs.ReadFile(subFS, path)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}

		// 闂傚倸鍊搁崐椋庣矆娓氣偓楠炴牠顢曢妶鍥╃厠闂佺粯鍨堕弸鑽ょ礊閺嵮岀唵閻犺櫣灏ㄩ崝鐔兼煛閸℃劕鈧洟濡撮幒鎴僵闁挎繂鎳嶆竟鏇熶繆閵堝洤啸闁稿绋撶划鏃囥亹閹烘垶妲梺鏂ユ櫅閸燁垱鍒婇幘顔界厽闁瑰鍎愰悞鐣岀磼婢舵劗鐣烘慨濠勭帛閹峰懘宕ㄦ繝鍌涙畼闂備浇鍋愰幊鎾存櫠閻ｅ苯鍨濋柡鍐ㄧ墛閸婅崵绱掑☉姗嗗剱闁哄應鏅犲娲传閸曨剙绐涙繝娈垮櫍椤ユ挻绔?MIME 缂傚倸鍊搁崐鎼佸磹妞嬪孩顐介柨鐔哄Т绾捐顭块懜闈涘Е闁轰礁顑囬幉鎼佸籍閸繄鐣?
		contentType := "application/octet-stream"
		switch {
		case strings.HasSuffix(fp, ".js"):
			contentType = "application/javascript; charset=utf-8"
		case strings.HasSuffix(fp, ".css"):
			contentType = "text/css; charset=utf-8"
		case strings.HasSuffix(fp, ".svg"):
			contentType = "image/svg+xml"
		case strings.HasSuffix(fp, ".png"):
			contentType = "image/png"
		case strings.HasSuffix(fp, ".jpg"), strings.HasSuffix(fp, ".jpeg"):
			contentType = "image/jpeg"
		case strings.HasSuffix(fp, ".woff2"):
			contentType = "font/woff2"
		case strings.HasSuffix(fp, ".woff"):
			contentType = "font/woff"
		}

		c.Data(http.StatusOK, contentType, data)
	})

	// vite.svg
	s.router.GET("/vite.svg", func(c *gin.Context) {
		data, _ := fs.ReadFile(subFS, "vite.svg")
		c.Data(http.StatusOK, "image/svg+xml", data)
	})

	// 婵犵數濮烽。钘壩ｉ崨鏉戠；闁规崘娉涚欢銈呂旈敐鍛殲闁稿顑夐弻锝呂熷▎鎯ф閺?
	s.router.GET("/", func(c *gin.Context) {
		data, _ := fs.ReadFile(subFS, "index.html")
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	// SPA 闂傚倸鍊峰ù鍥х暦閸偅鍙忕€规洖娲︽刊浼存煥閺囩偛鈧悂宕归崒鐐寸厵闁诡垳澧楅ˉ澶愭煛鐎ｂ晝绐旀慨濠冩そ椤㈡鍩€椤掑倻鐭撻柟缁㈠枟閸婂嘲鈹戦悩鍙夊闁绘挾鍠栭弻鐔兼焽閿曗偓閻忔盯鏌嶈閸忔﹢宕戦幘缁樷拺缂佸顑欓崕鎰版煙閻熺増鍠樼€规洘妞介崺鈧?
	s.router.NoRoute(func(c *gin.Context) {
		data, _ := fs.ReadFile(subFS, "index.html")
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})
}

func (s *Server) Run() error {
	return s.router.Run(s.cfg.ListenAddr)
}

// RunWithContext starts the server and shuts down gracefully when ctx is cancelled.
func (s *Server) RunWithContext(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.cfg.ListenAddr,
		Handler: s.router,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// ==================== 婵犵數濮烽弫鎼佸磻閻愬搫鍨傞柛顐ｆ礀缁犲綊鏌嶉崫鍕櫣闁活厽顨婇弻娑㈠箛閳轰礁顥嬪┑鐐村灟閸ㄥ湱绮绘导鏉戠閺夊牆澧界粔鍨箾?====================

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := c.GetHeader("Authorization")
		if tokenStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			c.Abort()
			return
		}

		// 缂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾閽樻繃銇勯弽顐汗闁逞屽墾缁犳垿鎮鹃敓鐘茬闁惧浚鍋嗛埀?"Bearer " 闂傚倸鍊搁崐椋庣矆娓氣偓楠炲鏁撻悩鑼唶闂佸憡绺块崕鎶芥儗閹剧粯鐓曢柟鎹愬皺閸斿秵銇?
		if len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
			tokenStr = tokenStr[7:]
		}

		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(s.cfg.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
			return
		}
		if temp2FA, ok := claims["temp_2fa"].(bool); ok && temp2FA {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "2fa verification required"})
			c.Abort()
			return
		}
		jti, ok := claims["jti"].(string)
		if !ok || jti == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session token"})
			c.Abort()
			return
		}

		// 婵犵數濮撮惀澶愬级鎼存挸浜炬俊銈勭劍閸欏繘鏌熺紒銏犳灍闁稿孩顨呴妴鎺戭潩閿濆懍澹曢梻?JTI (婵犵數濮烽弫鎼佸磻閻愬樊鐒芥繛鍡樻尭鐟欙箓鎮楅敐搴′簽闁崇懓绉电换娑橆啅椤旇崵鍑归梺绋块閿曨亪寮婚敐澶嬫櫜闁告侗鍨虫导鍥ㄧ箾閹寸偞灏紒澶婄秺瀵濡搁妷銏☆潔濠碘槅鍨拃锔界妤ｅ啯鈷?
		if !s.svc.ValidateSession(jti) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "session expired or invalid"})
			c.Abort()
			return
		}
		go s.svc.UpdateSessionActivity(jti)
		c.Set("user_id", claims["user_id"])
		c.Set("username", claims["username"])
		c.Set("role", claims["role"])
		c.Set("jti", jti)
		c.Next()
	}
}

// wsAuthMiddleware WebSocket 闂傚倸鍊峰ù鍥х暦閸偅鍙忛柟鎯板Г閳锋梻鈧箍鍎遍ˇ顖滃鐟欏嫮绠鹃柟瀛樼懃閻忣亪鏌涙惔鈽呭伐妞ゎ叀娉曢幑鍕瑹椤栨艾澹嬮梻浣哥－缁垰顫忚ぐ鎺懳﹂柛鏇ㄥ灠缁犲鏌涘Δ鍐ㄤ户濞寸娀绠栧娲箰鎼达絻鈧帡鏌涢悩鏌ュ弰婵犫偓?(闂傚倸鍊搁崐宄懊归崶顒€违闁逞屽墴閺屾稓鈧綆鍋呯亸浼存煏閸パ冾伃鐎殿喕绮欐俊姝岊槷婵℃彃鐗撳?query 闂傚倸鍊搁崐椋庣矆娓氣偓楠炲鏁撻悩鍐蹭画闂侀潧顦弲娑氬閸︻厽鍠愰柣妤€鐗嗙粭鎺撴叏鐟欏嫮鍙€闁哄矉缍佸顕€宕掗妶鍥уЪ婵＄偑鍊栧ú鐔哥閸洖钃熼柨婵嗘啒閻斿吋鍋傞幖绮光偓鎵挎洟姊?token)
func (s *Server) wsAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// WebSocket 婵犵數濮烽弫鎼佸磻閻愬搫鍨傞柛顐ｆ礀缁犱即鏌涘┑鍕姢闁活厽鎹囬弻锝夊閵忊晝鍔搁梺鍝勵儎閻掞箓濡甸崟顖氬嵆婵°倐鍋撳ù婊勫劤铻栭柣姗€娼ф禒锕傛煕閺冣偓閻楃娀鐛箛娑樺窛閻庢稒锚閻у嫭绻濋姀锝嗙【妞ゆ垶鍔欏畷顒勫醇閺囩啿鎷洪柣鐘叉搐瀵爼宕径瀣ㄤ簻妞ゆ劑鍩勫Ο鈧梺璇″灠閻倿銆佸▎鎾崇鐟滃瞼绮?Header闂傚倸鍊搁崐鐑芥倿閿旈敮鍋撶粭娑樻噽閻瑩鏌熺€电浠ч梻鍕閺岋繝宕橀妸銉㈠亾瑜版帪缍栭柡鍥╁枂娴滄粓鏌熼幆褍鑸归柣蹇婃櫇缁辨帡鍩€椤掑嫬纾奸柣鎰嚟閸樺崬顪冮妶鍡楀Ё缂佹彃娼￠幆灞界暆閸曨剛鍘?query 闂傚倸鍊搁崐椋庣矆娓氣偓楠炲鏁撻悩鍐蹭画闂侀潧顦弲娑氬閸︻厽鍠愰柣妤€鐗嗙粭鎺撴叏鐟欏嫮鍙€闁哄矉缍佸顒勫垂椤旇棄鈧垶姊洪幖鐐测偓鏍ь潖婵犳艾鐒垫い鎺戝€归崵鈧柣搴㈠嚬閸樺ジ鈥﹂崹顔ョ喖鎮℃惔锝囩摌?token
		tokenStr := c.Query("token")
		if tokenStr == "" {
			tokenStr = c.GetHeader("Authorization")
			if len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
				tokenStr = tokenStr[7:]
			}
		}

		if tokenStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			c.Abort()
			return
		}

		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(s.cfg.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
			return
		}
		if temp2FA, ok := claims["temp_2fa"].(bool); ok && temp2FA {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "2fa verification required"})
			c.Abort()
			return
		}
		jti, ok := claims["jti"].(string)
		if !ok || jti == "" || !s.svc.ValidateSession(jti) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "session expired or invalid"})
			c.Abort()
			return
		}
		c.Set("user_id", claims["user_id"])
		c.Set("username", claims["username"])
		c.Set("role", claims["role"])
		c.Set("jti", jti)
		c.Next()
	}
}

func (s *Server) viewerWriteBlockMiddleware() gin.HandlerFunc {
	// 婵犵數濮烽弫鎼佸磻閻愬搫鍨傞柛顐ｆ礀缁犲綊鏌嶉崫鍕偓濠氥€呴崣澶岀闁糕剝顨夐鐔烘喐鎼粹槅鍤楅柛鏇ㄥ幒濞岊亪鏌熼鍡楀缁扁晠姊婚崒娆戝妽閻庣瑳鍏炬稒鎷呴懖婵囩洴瀹曠喖顢涘☉妯圭暗闂備礁鎼ú銊︽叏妞嬪骸顥氬┑鍌氭啞閻撶喐绻涢崱妤冪闁革絿鏅惀顏堝箚瑜庨崑銉╂煛瀹€鈧崰鏍€佸▎鎴炲厹鐎瑰嫭婢橀～鐘绘⒒娴ｅ憡璐￠柟铏尵閳ь剚鐭崡鎶藉Υ娴ｇ硶妲堟慨妤€妫涢崣鍡涙⒑閸涘﹥澶勯柛鎾村哺椤?(viewer 婵犵數濮烽弫鎼佸磻閻愬搫鍨傞悹杞拌濞兼牕鈹戦悩瀹犲缂佹劖顨婇獮鏍庨鈧埀顒佸灦缁傚秷銇愰幒鎾跺幈濠电偛妫楀ù姘ｉ悜妯镐簻闁冲搫鍟崢鎾煛鐏炵偓绀冪€垫澘瀚埥澶愬閳藉棌鍋撳澶嬧拺閻犲洠鈧磭浠梺鑽ゅ枂閸庨潧锕?
	personalPaths := map[string]bool{
		"/api/change-password":     true,
		"/api/profile":             true,
		"/api/profile/2fa/enable":  true,
		"/api/profile/2fa/verify":  true,
		"/api/profile/2fa/disable": true,
		"/api/sessions/:id":        true,
		"/api/sessions/others":     true,
	}

	return func(c *gin.Context) {
		// 闂傚倸鍊搁崐鐑芥嚄閸洍鈧箓宕奸姀鈥冲簥闁诲函缍嗛埀顑惧灩缂嶅﹤鐣烽崼鏇ㄦ晢濠㈣泛顑嗗▍鎾绘⒒婵犲骸浜滄繛璇у缁瑩骞嬮悩鍏哥瑝闂侀潧顦弲婊堟偂閸愵亝鍠愭繝濠傜墕缁€鍫ユ煟閺傛娈犻柛?GET 闂傚倸鍊峰ù鍥х暦閸偅鍙忛柡澶嬪殮濞差亶鏁囬柕蹇曞Х閸濇姊绘笟鍥у缂佸鏁诲畷?(闂傚倸鍊峰ù鍥х暦閸偅鍙忛柡澶嬪殮濞差亜惟鐟滃秹宕瑰┑鍥ヤ簻闁哄稁鍋勬禒婊呯磼閻樿尙绉烘慨濠冩そ瀹曞綊顢氶崨顓炲濠?
		if c.Request.Method == "GET" {
			c.Next()
			return
		}

		// 闂傚倸鍊搁崐鐑芥嚄閸洍鈧箓宕奸姀鈥冲簥闁诲函缍嗛埀顑惧灩缂嶅﹤鐣烽崼鏇ㄦ晢濠㈣泛顑嗗▍鎾绘⒒婵犲骸浜滄繛璇х畱椤繑绻濆顒傚幒闂佸搫鍟犻崑鎾绘煏閸パ冾伂缂佺姵鐩獮姗€鎼归锝庡敼闂傚倷鐒﹀鎸庣濞嗘垵鍨濇繛鍡樻尭缁犳牜鎲搁悧鍫濈瑨缂佺姵鐩弻娑㈩敃閿濆棛顦ㄥ銈呴缁夌懓顫忛搹鐟板闁哄洨鍠愰悵鏇犵磽娴ｅ壊鍎愰柟鎼佺畺瀹曟岸骞掗幘鍓佺槇濠殿喗锕╅崢钘夆枍閺嶎厽鈷戦柛娑橈攻婢跺嫰鏌涢幘瀵告噰闁糕斁鍋撳銈嗗笒閸婂憡绂掗柆宥嗙厵妞ゆ梹顑欏鎰磼濡ゅ啫鏋涙い銏＄☉椤繈宕ｅΟ鍝勵嚙闂?
		if personalPaths[c.FullPath()] {
			c.Next()
			return
		}

		role, exists := c.Get("role")
		if exists {
			if r, ok := role.(string); ok && r == "viewer" {
				c.JSON(http.StatusForbidden, gin.H{"error": "只读用户无权执行此操作"})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// ==================== 闂傚倸鍊峰ù鍥х暦閸偅鍙忛柟鎯板Г閳锋梻鈧箍鍎遍ˇ顖滃鐟欏嫮绠鹃柟瀛樼懃閻忣亪鏌涙惔鈽呭伐妞ゎ厼娼￠幊婊堟濞戞﹩娼撶紓鍌欒兌婵灚绻涙繝鍥ц摕闁哄洨鍠庣欢鐐烘煕椤愶絿绠撳┑顔兼捣缁?====================

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (s *Server) login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := s.svc.ValidateUser(req.Username, req.Password)
	if err != nil {
		// 闂傚倸鍊峰ù鍥х暦閸偅鍙忛柟鎯板Г閸婂潡鏌ㄩ弴妤€浜惧銈庡幖閻忔繆鐏掗梺鍏肩ゴ閺呮繈藝椤栫偞鍊垫鐐茬仢閸旀碍銇勯敂鍨祮鐎规洘娲栭悾婵嬪礋椤掆偓娴狀垶姊洪幖鐐插姶濞存粍绮撻幆鍐惞閸︻厾锛滄繝銏ｆ硾閿曪附绂掗姀掳浜滄い鎾跺仦閸嬨儲顨ラ悙鍙夘棦妤犵偛绉归、娆撳礈瑜嶇€氬酣姊?
		s.svc.LogOperation(0, req.Username, "login", "user", 0, "login failed", c.ClientIP(), c.GetHeader("User-Agent"), "failed")
		RecordLoginAttempt(false)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if !user.Enabled {
		s.svc.LogOperation(user.ID, user.Username, "login", "user", user.ID, "account disabled", c.ClientIP(), c.GetHeader("User-Agent"), "failed")
		c.JSON(http.StatusForbidden, gin.H{"error": "account is disabled"})
		return
	}

	// 濠电姷鏁告慨鐑姐€傞挊澹╋綁宕ㄩ弶鎴狅紱闂侀€炲苯澧撮柡灞剧〒閳ь剨缍嗛崑鍛暦瀹€鍕厸鐎光偓鐎ｎ剛锛熸繛瀵稿婵″洭骞忛悩璇茬闁圭儤鍤╅弴銏♀拻濞达絼璀﹂悞鐐亜閹存繃顥㈤柟顔ㄥ喚鐓ラ柛娑卞幖閻忓﹦绱撻崒娆戝妽閽冭京绱撳鍛枠闁哄本娲樼换娑㈠Χ閸屾矮澹曢梻浣瑰濞叉牠宕愯ぐ鎺撳亗闁哄洨鍠愰崣蹇斾繆椤栨碍鎯堥柍閿嬫閺岋綁鏁愭径澶嬪枤闂佸搫鏈惄顖炵嵁鐎ｎ喖妫橀柛顭戝枓閹枫倖淇婇悙顏勨偓鎴﹀礉婵犲洤纾块梺顒€绉甸崑鈺傜節闂堟侗鍎忕紒鈧崘鈹夸簻闊洦鎸绘刊鍏肩箾閸忓吋顥堟慨濠勭帛閹峰懘鎮烽柇锕€娈濈紓鍌欑椤戝棝宕归崸妤€绠栫憸鏂跨暦閸楃偐妲堥柡宥冨€曟禍楣冩煕濞戙垹浜版俊顖氬閹鈻撻崹顔界亾濡炪値鍘奸悧鎾诲灳閿曞倸惟闁宠桨绶氶崬鍫曟⒑缂佹ɑ鐓ュ褍娴烽弫顔尖槈閵忊檧鎷洪梺鐓庮潟閸婃洟寮搁幋鐐电闁告瑥顦悘鈺傜箾閸℃劕鐏插┑鈥崇埣瀹曞爼鈥﹂幋鐐电◥?
	if s.svc.IsEmailVerificationRequired() && !user.EmailVerified && user.Email != nil && *user.Email != "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "email not verified", "code": "EMAIL_NOT_VERIFIED"})
		return
	}

	// 濠电姷鏁告慨鐑姐€傞挊澹╋綁宕ㄩ弶鎴狅紱闂侀€炲苯澧撮柡灞剧〒閳ь剨缍嗛崑鍛暦瀹€鍕厸鐎光偓鐎ｎ剛锛熸繛瀵稿婵″洭骞忛悩璇茬闁圭儤鍩堝銉モ攽閻樻鏆柍褜鍓欓崯璺ㄧ棯瑜旈弻鐔碱敊閻撳簶鍋撻幖浣瑰仼闁绘垼妫勫敮闂佸啿鎼崐鐟扳枍閸ヮ剚鈷戦梺顐ゅ仜閼活垱鏅剁€涙﹩娈介柣鎰级椤ョ偤鏌熼娑欘棃濠碘剝鎮傞弫鍐焵椤掍胶顩锋繛宸簼閳锋垿姊婚崼鐔剁繁闁绘帡绠栭弻娑欑節閸愮偓鐤侀悗?2FA
	if user.TwoFactorEnabled {
		tempToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id":  user.ID,
			"username": user.Username,
			"temp_2fa": true,
			"exp":      time.Now().Add(5 * time.Minute).Unix(),
		})

		tempTokenString, err := tempToken.SignedString([]byte(s.cfg.JWTSecret))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate temp token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"requires_2fa": true,
			"temp_token":   tempTokenString,
		})
		return
	}

	s.loginLimiter.Reset(c.ClientIP())
	RecordLoginAttempt(true)

	// 闂傚倸鍊搁崐椋庣矆娓氣偓楠炴牠顢曢埛姘そ婵¤埖寰勭€ｎ亙妲愰梻渚€娼ц墝闁哄懏鐩幏鎴︽偄鐏忎焦鏂€闂佺粯锚瀵埖寰勯崟顖涚厱閹艰揪绱曢悞鎼佹煙椤旇偐绉虹€规洖鐖奸、鏃堝礋椤撶噥鍤勭紓鍌氬€风粈渚€藝椤栫偞鍊舵繝闈涳工缁插綊姊绘担瑙勫仩闁稿氦宕靛濠囨嚍閵夛附鐝烽梺鍦帛瀹?
	s.svc.UpdateUserLoginInfo(user.ID, c.ClientIP())

	// 闂傚倸鍊峰ù鍥х暦閸偅鍙忛柟鎯板Г閸婂潡鏌ㄩ弴妤€浜惧銈庡幖閻忔繆鐏掗梺鍏肩ゴ閺呮繈藝椤栫偞鍊垫鐐茬仢閸旀碍銇勯敂鍨祮鐎规洘娲栭悾婵嬪礋椤掆偓娴狀垶姊洪幖鐐插姶濞存粍绮撻幆鍐惞閸︻厾锛滄繝銏ｆ硾閿曘儵宕戦妷鈺傜厸鐎光偓閳ь剟宕伴幘璇茬劦妞ゆ帊鑳堕埊鏇㈡煥濮橆兘鏀芥い鏂垮悑閹兼劙鏌?
	s.svc.LogOperation(user.ID, user.Username, "login", "user", user.ID, "login success", c.ClientIP(), c.GetHeader("User-Agent"), "success")

	// 闂傚倸鍊搁崐鐑芥倿閿曞倹鍎戠憸鐗堝笒閸ㄥ倸鈹戦悩瀹犲缂佹劖顨婇弻鐔兼偋閸喓鍑￠梺?JWT with JTI
	jti := uuid.New().String()
	expiresAt := time.Now().Add(24 * time.Hour)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"role":     user.Role,
		"jti":      jti,
		"exp":      expiresAt.Unix(),
	})

	tokenString, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// 闂傚倸鍊搁崐椋庣矆娓氣偓楠炲鏁嶉崟顒佹濠德板€曢崯浼存儗濞嗘挻鐓欓悗鐢殿焾鍟哥紒鎯у綖缁瑩寮婚悢鍏煎€锋い鎺嶈兌缁嬪洤顪冮妶鍡樼濡炲瓨鎮傞獮鍫ュΩ閵夊海鍠栭幖褰掓儌閸濄儳顦┑鐘殿暯濡插懘宕规导瀛樺亱闁规崘顕х粻鏍偡濞嗗繐顏┑顖涙尦閹嘲鈻庤箛鎿冧紑闂佸憡鍩婄槐鏇犳?
	if err := s.svc.CreateUserSession(user.ID, jti, c.ClientIP(), c.GetHeader("User-Agent"), expiresAt); err != nil {
		s.svc.LogOperation(user.ID, user.Username, "session_create", "user_session", 0, fmt.Sprintf("failed to create session: %v", err), c.ClientIP(), c.GetHeader("User-Agent"), "failed")
	}

	c.JSON(http.StatusOK, gin.H{
		"token": tokenString,
		"user": gin.H{
			"id":                user.ID,
			"username":          user.Username,
			"email":             user.Email,
			"role":              user.Role,
			"email_verified":    user.EmailVerified,
			"password_changed":  user.PasswordChanged,
			"plan":              user.Plan,
			"plan_id":           user.PlanID,
			"plan_start_at":     user.PlanStartAt,
			"plan_expire_at":    user.PlanExpireAt,
			"plan_traffic_used": user.PlanTrafficUsed,
		},
	})
}

// ==================== 闂傚倸鍊搁崐鐑芥倿閿曗偓椤啴宕归鍛姺闂佺鍕垫當缂佲偓婢跺备鍋撻獮鍨姎妞わ富鍨跺浼村Ψ閿斿墽顔曢梺鐟邦嚟閸婃垵顫濈捄鍝勫殤闂佸搫绋侀崢浠嬫偂韫囨挴鏀介柣鎰皺娴犮垽鏌涢弮鈧畝鎼佸蓟閿熺姴纾兼俊銈傚亾濞存粎鍋撶换婵嬫偨闂堟稐娌梺鎼炲妼閻栧ジ鐛幇鏉跨濞达絽婀遍崢鎰版⒑閹稿海绠撴い锔垮嵆瀹曟垵螣閼姐倗顔曢梺绯曞墲閿氶柣蹇ョ畵閹?====================

// RegisterRequest 濠电姷鏁告慨鐑藉极閹间礁纾绘繛鎴旀嚍閸ヮ剦鏁囬柕蹇曞Х椤︻噣鎮楅崗澶婁壕闂佸憡娲﹂崑澶愬春閻愮儤鈷戝ù鍏肩懅閸掍即鏌￠崼顐㈠闁逞屽墯绾板秴鐣濈粙娆炬綎婵炲樊浜滈崹鍌涖亜閺囩偞鍣归柛鎾冲暱閳规垿鎮╅顫?
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

func (s *Server) register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := s.svc.RegisterUser(req.Username, req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !user.EmailVerified && user.VerificationToken != "" {
		go s.sendVerificationEmail(user)
	}

	// 闂傚倸鍊峰ù鍥х暦閸偅鍙忛柟鎯板Г閸婂潡鏌ㄩ弴妤€浜惧銈庡幖閻忔繆鐏掗梺鍏肩ゴ閺呮繈藝椤栫偞鍊垫鐐茬仢閸旀岸鏌熼崘宸█鐎殿喗鎮傚浠嬵敇閻斿搫甯惧┑鐘垫暩閸嬫盯鎮樺┑瀣婵鍩栭悡娑㈡煕鐏炵虎娈斿ù婊堢畺濮婂宕掑▎鎺戝帯闂佺娅曢幑鍥箖濡　鏀介柛顐犲灮閻撳鎮楃憴鍕婵炲眰鍔庣划?
	s.svc.LogOperation(user.ID, user.Username, "register", "user", user.ID, "user registered", c.ClientIP(), c.GetHeader("User-Agent"), "success")

	c.JSON(http.StatusOK, gin.H{
		"message":            "Registration successful",
		"email_verification": !user.EmailVerified,
	})
}

func (s *Server) verifyEmail(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := s.svc.VerifyEmail(req.Token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	go s.sendWelcomeEmail(user)

	c.JSON(http.StatusOK, gin.H{"message": "Email verified successfully"})
}

// ForgotPasswordRequest 闂傚倸鍊搁崐鎼佲€﹂鍕；闁告洦鍊嬪ú顏呮櫇闁稿本銇涢崑鎾绘晝閸屾岸鍞堕梺闈涱樈閸犳寮查埡鍛拺闁硅偐鍋涢崝妤呮煛閸屾瑧绐旂€规洘鍨块獮妯肩磼濮楀棙顥堟繝鐢靛仦閸ㄥ爼鎮烽姣兼盯濡舵径瀣ф嫼缂傚倷鐒﹂…鍥ㄦ櫠椤掑倻纾兼い鏂裤偢閸欏嫭顨ラ悙鑼闁诡喒鏅濈槐鎺懳熼悡搴℃辈闂備浇顕ч崙鐣岀礊閸℃稑绀堟慨妯挎硾缁€?
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

func (s *Server) forgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, token, err := s.svc.RequestPasswordReset(req.Email)
	if err != nil {
		// 婵犵數濮烽弫鎼佸磻閻愬搫鍨傞柛顐ｆ礀缁犳彃霉閿濆懎顥忔繛瀛樼墵閺屾洘绻涜濡鐨梻鍌欑劍鐎笛呯矙閹寸姭鍋撳鈧崶褏锛涢梺鐟板⒔缁垶宕戦敓鐘斥拺妞ゆ挶鍔戝顔剧箔閹达附鈷掑ù锝呮啞閸熺偤鏌ｉ悢鏉戝姢闁瑰箍鍨归～婊堝焵椤掆偓椤曪綁顢曢姀鈺佹倯闂佹悶鍎婚梽鍕礋閸愵喗鈷戦柛蹇氬亹閵堟挳鏌￠崨顔剧疄妤犵偛顦抽妵鎰板箳閹绢垱瀚奸梻浣告啞缁诲倻鈧凹鍙冭棢闁绘劗鍎ら悡娆戠磼鐎ｎ亞浠㈤柡鍡涗憾閺屻劑寮村Ο铏逛紙閻庤娲栭妶绋款嚕閹绢喗鍋勯柛婵嗗妤旀繝纰夌磿閸嬫垿宕愰弽褜娼栧┑鐘崇閹偤骞栧ǎ顒€濡奸柤绋跨秺閺岀喖姊荤€电濡介梺缁樻尰閻╊垶寮诲☉銏犲嵆闁靛繆鍓濋柨顓㈡⒑鐠囪尙绠冲┑鐐╁亾闂佸搫鐬奸崰鏍€佸☉妯锋瀻闁圭儤鍨电敮顖炴⒒娓氣偓閳ь剛鍋涢懟顖涙櫠椤栨粎纾奸悗锝庡亜濞搭噣鏌℃担鍓插剰闁宠鍨块幃鈺咁敃椤厼顥?
		c.JSON(http.StatusOK, gin.H{"message": "If the email exists, a reset link has been sent"})
		return
	}

	go s.sendPasswordResetEmail(user, token)

	c.JSON(http.StatusOK, gin.H{"message": "If the email exists, a reset link has been sent"})
}

// ResetPasswordRequest 闂傚倸鍊搁崐鎼佸磹閻戣姤鍊块柨鏇氶檷娴滃綊鏌涢幇鍏哥敖闁活厽鎹囬弻娑㈩敃閿濆棛顦ㄩ梺绋款儛娴滎亪寮诲☉銏犵労闁告劦浜濋崰娑㈡⒑缁嬫鍎愰柟鐟版搐铻為柛鎰╁妷濡插牊绻涢崱妯曟垿鏁撻妷鈺傗拻濞达綀娅ｇ敮娑㈡煟濡や緡娈橀柛鎺撳笚閹棃濡搁妷褜鍞甸梻浣虹帛椤洨鍒掗鐐村亗闁靛鏅滈崐鐢告煥濠靛棝顎楀褜鍨堕弻?
type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

func (s *Server) resetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.svc.ResetPassword(req.Token, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password reset successfully"})
}

func (s *Server) getRegistrationStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"enabled":            s.svc.IsRegistrationEnabled(),
		"email_verification": s.svc.IsEmailVerificationRequired(),
	})
}

func (s *Server) adminVerifyUserEmail(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if err := s.svc.UpdateUser(uint(id), map[string]interface{}{
		"email_verified":     true,
		"verification_token": "",
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) resendVerificationEmail(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	token, err := s.svc.ResendVerificationEmail(uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, _ := s.svc.GetUser(uint(id))
	if user != nil {
		user.VerificationToken = token
		go s.sendVerificationEmail(user)
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// 闂傚倸鍊搁崐鎼佸磹閻戣姤鍊块柨鏇氶檷娴滃綊鏌涢幇鍏哥敖闁活厽鎹囬弻娑㈩敃閿濆棛顦ㄩ梺绋款儛娴滎亪寮诲☉銏犵労闁告劦浜栨慨鍥⒑缁嬫鍎嶉柛鏃€鍨垮濠氬即閻旇櫣鐦堥棅顐㈡处濞叉粓寮抽悩缁樷拺缂備焦蓱鐏忕増绻涢懠顒€鏋庢い顐㈢箰鐓ゆい蹇撳妤犲洭鏌ｉ悩鍏呰埅闁告捁绮鹃ˇ鎶芥煏?
func (s *Server) resetUserQuota(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if err := s.svc.ResetUserQuota(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) sendVerificationEmail(user *model.User) {
	if user.Email == nil || *user.Email == "" {
		return
	}
	emailSender := s.getEmailSender()
	if emailSender == nil {
		return
	}
	emailSender.SendVerificationEmail(*user.Email, user.Username, user.VerificationToken)
}

func (s *Server) sendPasswordResetEmail(user *model.User, token string) {
	if user.Email == nil || *user.Email == "" {
		return
	}
	emailSender := s.getEmailSender()
	if emailSender == nil {
		return
	}
	emailSender.SendPasswordResetEmail(*user.Email, user.Username, token)
}

func (s *Server) sendWelcomeEmail(user *model.User) {
	if user.Email == nil || *user.Email == "" {
		return
	}
	emailSender := s.getEmailSender()
	if emailSender == nil {
		return
	}
	emailSender.SendWelcomeEmail(*user.Email, user.Username)
}

// getEmailSender 闂傚倸鍊搁崐椋庣矆娓氣偓瀹曘儳鈧綆鍠栫壕鍧楁煙閹増顥夐幖鏉戯躬閺屻倝鎳濋幍顔肩墯婵炲瓨绮岀紞濠囧蓟濞戙垹唯妞ゆ梻鍘ч～顏堟⒑缂佹ê绗╁┑顔哄€濇俊鐢稿礋椤栨氨鐫勯梺绋挎湰缁瞼绮垾鎰佹富闁靛牆妫楁慨鍐磼椤旂晫鎳囩€殿喛顕ч埥澶娢熼柨瀣偓濠氭⒑瑜版帒浜伴柛鎾寸⊕鐎电厧鐣濋崟顑芥嫼闂備緡鍋嗛崑娑㈡嚐椤栫偛鍌ㄩ柛婵勫劤绾惧ジ鏌嶈閸撴岸骞忛崨顖氬闁哄洨鍠撻埀?
func (s *Server) getEmailSender() *notify.EmailSender {
	// 闂傚倸鍊搁崐椋庣矆娓氣偓楠炴牠顢曢妶鍡椾粡濡炪倖鍔х粻鎴犲閸ф鐓曟繛鍡楁禋濡叉椽鏌?SMTP 缂傚倸鍊搁崐鎼佸磹妞嬪孩顐介柨鐔哄Т绾捐顭块懜闈涘Е闁轰礁顑囬幉鎼佸籍閸繄鐣哄┑掳鍊曢幊蹇涘磹婵犳碍鐓犻柟顓熷笒閸旀岸鏌涢幙鍐ㄥ籍婵﹥妞藉畷銊︾節閸愵煈妲遍梻浣稿閻撳牊绂嶉鍕殾闁硅揪绠戠粻濠氭煙妫颁胶鍔嶉柛宥囨暬閺屸剝寰勬繝鍕暤闂佸搫鎳忕粙鎴︻敋閿濆惟闁靛鍨洪弬鈧梻浣虹帛閸旀洟鎮洪妸褌鐒婇梻鍫熺▓閺€浠嬫煟閹般劍娅呭ù婊勫劤閳?
	channels, err := s.svc.ListNotifyChannels()
	if err != nil {
		return nil
	}

	for _, ch := range channels {
		if (ch.Type == "smtp" || ch.Type == "email") && ch.Enabled {
			var smtpConfig model.SMTPConfig
			if err := json.Unmarshal([]byte(ch.Config), &smtpConfig); err != nil {
				continue
			}
			siteName := s.svc.GetSiteConfig(model.ConfigSiteName)
			siteURL := s.svc.GetSiteConfig(model.ConfigSiteURL)
			if siteName == "" {
				siteName = "GOST Panel"
			}
			return notify.NewEmailSender(&smtpConfig, siteName, siteURL)
		}
	}
	return nil
}

func (s *Server) getStats(c *gin.Context) {
	stats, err := s.svc.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (s *Server) healthCheck(c *gin.Context) {
	// 濠电姷鏁告慨鐑姐€傞挊澹╋綁宕ㄩ弶鎴狅紱闂侀€炲苯澧撮柡灞剧〒閳ь剨缍嗛崑鍛暦瀹€鍕厸鐎光偓鐎ｎ剛锛熸繛瀵稿婵″洭骞忛悩璇茬闁圭儤鍩堝銉╂⒒閸屾瑧顦﹂柟纰卞亜铻炴繛鎴欏灩缁愭鏌″搴″箻鐎规挷绶氶弻鐔告綇閹呮В闂佽桨绀侀敃顏堝蓟閵堝悿鍦偓锝庡亝閻濇洟姊哄Ч鍥у闁搞劌娼″濠氬焺閸愩劎绐為梺绯曞墲钃遍柣婵囩墵濮婅櫣绮欏▎鎯у壋闂佺顑囬崰鏍ь嚕?
	dbOk := true
	dbStatusStr := "ok"
	if err := s.svc.Ping(); err != nil {
		dbStatusStr = "error"
		dbOk = false
	}
	UpdateDBStatus(dbOk)

	// 闂傚倸鍊搁崐椋庣矆娓氣偓瀹曘儳鈧綆鍠栫壕鍧楁煙閹増顥夐幖鏉戯躬閺屻倝鎳濋幍顔肩墯婵炲瓨绮岀紞濠囧蓟濞戙垹唯妞ゆ梻鍘ч～鈺呮⒑閸濆嫬顏ラ柛搴ｆ暬瀵鏁愭径濠勵吅闂佺粯鍔曞Λ娆撳垂閸ф宓侀煫鍥ㄦ⒐缂嶅洭鏌嶉崫鍕殶闁挎稒绻冪换娑欐綇閸撗冨煂闂佸湱鈷堥崑濠傤嚕缁嬪簱鏋庨柟鎵虫櫃缁?
	stats, _ := s.svc.GetStats()
	nodeCount := 0
	onlineNodes := 0
	clientCount := 0
	onlineClients := 0
	userCount := 0
	if stats != nil {
		nodeCount = stats.TotalNodes
		onlineNodes = stats.OnlineNodes
		clientCount = stats.TotalClients
		onlineClients = stats.OnlineClients
		userCount = stats.TotalUsers
	}

	// 闂傚倸鍊搁崐椋庣矆娓氣偓楠炴牠顢曢埛姘そ婵¤埖寰勭€ｎ亙妲愰梻渚€娼ц墝闁哄懏鐩幏?Prometheus 闂傚倸鍊搁崐椋庣矆娴ｉ潻鑰块梺顒€绉埀顒婄畵瀹曠厧顭垮┑鍥ㄣ仢闁轰礁鍟村畷鎺戔槈濡懓鍤?
	UpdateNodeMetrics(nodeCount, onlineNodes)
	UpdateClientMetrics(clientCount, onlineClients)
	UpdateUserMetrics(userCount)

	c.JSON(http.StatusOK, gin.H{
		"status":         "ok",
		"database":       dbStatusStr,
		"version":        CurrentAgentVersion,
		"nodes":          nodeCount,
		"online_nodes":   onlineNodes,
		"clients":        clientCount,
		"online_clients": onlineClients,
		"users":          userCount,
	})
}

// allowedScripts 闂傚倸鍊搁崐鐑芥嚄閸洍鈧箓宕奸姀鈥冲簥闁诲函缍嗛埀顑惧灩缂嶅﹤鐣烽崼鏇ㄦ晢濠㈣泛顑嗗▍鎾绘⒒婵犲骸浜滄繛璇у缁瑩骞掑Δ鈧壕璺ㄢ偓骞垮劚椤︿即鎮￠弴銏＄厸闁搞儯鍎辨俊濂告煛鐎ｎ偆鈽夋い顓℃硶閹叉挳宕熼鐐╂嫲闂備礁鎼惌澶屾崲濠靛棛鏆﹀┑鍌氬閺佸啴鏌曡箛鏇烆€屾繛鑲╁亾缁绘繈鎮介棃娑楃捕闂佺灏欓崑銈呯暦閹偊妯侀悶姘箞閺岋絾鎯旈敍鍕殯闂佺閰ｆ禍鍫曠嵁婵犲啯鍎熼柨婵嗘川閻?(闂傚倸鍊搁崐鎼佸磹閹间礁纾归柟闂寸閻ゎ喗銇勯幇鈺佺労闁搞倖娲熼弻娑㈩敃閿濆棗顦╅梺杞扮濡瑧鎹㈠☉銏犵闁绘劖顔栭弳锟犳倵鐟欏嫭绀€妞わ缚鍗虫俊鐢稿礋椤栨艾鍞ㄩ梺闈涱煭婵″洤鈻撻妶鍥╃＝濞达絿鎳撴慨澶愭煕鐎ｃ劌鈧洟锝炶箛鎾佹椽顢斿鍡樻珜闂備線鈧偛鑻晶鎵磼椤旇姤顥堢€殿喖鐖奸獮瀣敇閻愬瓨鐝?
var allowedScripts = map[string]bool{
	"install-node.sh":    true,
	"install-client.sh":  true,
	"install-node.ps1":   true,
	"install-client.ps1": true,
}

// serveInstallScript returns a handler that serves install scripts
func (s *Server) serveInstallScript(scriptName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 婵犵數濮撮惀澶愬级鎼存挸浜炬俊銈勭劍閸欏繘鏌熺紒銏犳灍闁稿孩顨呴妴鎺戭潩閿濆懍澹曢梻浣筋嚃閸垶鎮為敃鈧銉╁礋椤栨氨鐤€濡炪倖鎸鹃崑鐐烘偩缂佹绡€婵炲牆鐏濋弸鐔兼煥閺囨娅婄€规洏鍨虹粋鎺斺偓锝庝簽椤斿棝姊绘笟鍥у缂佸鏁婚幃锟犲即閻旂寮垮┑顔筋殔濡鏅堕幍顔瑰亾濞戞瑧绠撻柍瑙勫灴閹晠顢曢～顓烆棜濠碉紕鍋戦崐鏇犳崲閹扮増鍋嬪┑鐘叉搐绾捐法鈧厜鍋撻柛鏇ㄥ墰閸欏啫鈹戦埥鍡楃仧閻犫偓閿曗偓鍗辩憸鐗堝笚閻撴洖鈹戦悩鎻掓殭闁崇粯娲熼弻锛勪沪閸撗€妲堥柧缁樼墪闇夐柨婵嗘噺鐠愶繝鏌ㄥ☉娆戞创婵?(闂傚倸鍊搁崐鎼佸磹閹间礁纾归柟闂寸閻ゎ喗銇勯幇鈺佺労闁搞倖娲熼弻娑㈩敃閿濆棗顦╅梺杞扮濡瑧鎹㈠☉銏犵闁绘劖顔栭弳锟犳倵鐟欏嫭绀€妞わ缚鍗虫俊鐢稿礋椤栨艾鍞ㄩ梺闈涱煭婵″洤鈻撻妶鍥╃＝濞达絿鎳撴慨澶愭煕鐎ｃ劌鈧洟锝炶箛鎾佹椽顢斿鍡樻珜闂備線鈧偛鑻晶鎵磼椤旇姤顥堢€殿喖鐖奸獮瀣敇閻愬瓨鐝?
		if !allowedScripts[scriptName] {
			c.JSON(http.StatusForbidden, gin.H{"error": "script not allowed"})
			return
		}

		// Try multiple paths
		paths := []string{
			filepath.Join("scripts", scriptName),
			filepath.Join(".", "scripts", scriptName),
		}

		var scriptPath string
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				scriptPath = p
				break
			}
		}

		if scriptPath == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "script not found"})
			return
		}

		content, err := os.ReadFile(scriptPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Set content type based on extension
		if filepath.Ext(scriptName) == ".ps1" {
			c.Header("Content-Type", "text/plain; charset=utf-8")
		} else {
			c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
		}
		c.Header("Content-Disposition", "inline; filename="+scriptName)

		c.String(http.StatusOK, string(content))
	}
}

// serveClientScript 闂傚倸鍊搁崐鎼佸磹妞嬪孩顐介柨鐔哄Т绾惧鏌涘☉鍗炴灓闁崇懓绉归弻褑绠涘鍏肩秷閻?token 闂傚倸鍊搁崐椋庣矆娴ｉ潻鑰块弶鍫氭櫅閸ㄦ繃銇勯弽顐粶缂佲偓婢舵劖鐓欓柣鎴炆戦埛鎰亜閹邦亞鐭欓柡宀嬬秮婵偓闁靛繆鏅滃В宀勬⒑缁嬫鍎愰柟鐟版喘閹即顢氶埀顒€鐣疯ぐ鎺濇晩闁告瑣鍎冲Λ顖炴⒒閸屾瑨鍏岀痪顓炵埣瀵彃顭ㄩ崨顖滅厯闂佽宕樺▔娑㈠垂濠靛鐓冮柛婵嗗婵ジ鏌℃担鍝バｉ柟渚垮妼铻ｉ柛褎顨呴幗闈涒攽閻愯尙鍨抽柛銉ｅ妿閸樹粙姊虹紒妯荤叆濠⒀冮叄钘濋柨鏇炲€归悡娆撴煣韫囷絽浜濈€规洖鐬奸埀顒冾潐濞叉鍒掑鍥╃处闁伙絽鐬奸惌娆撴煕椤垵鏋ょ憸?(闂傚倸鍊搁崐鐑芥嚄閸洍鈧箓宕煎婵堟嚀椤繄鎹勯搹璇℃Ф婵犵數鍋涘Λ娆撳垂閸洩缍栭柡鍥ュ灪閻撴瑩寮堕崼婵嗏挃闁伙綀浜槐鎺楁偐閸愭祴鏋欓梺鍝勬湰濞叉ê顕ラ崟顖氶唶婵犻潧妫楅～鎾剁磽? Agent 濠电姷鏁告慨鐑姐€傞挊澹╋綁宕ㄩ弶鎴濈€銈呯箰閻楀棝鎮為崹顐犱簻闁瑰搫妫楁禍鍓х磼閸撗嗘闁告ɑ鍎抽埥澶愭偨缁嬭法鍔?
func (s *Server) serveClientScript(c *gin.Context) {
	token := c.Param("token")

	client, err := s.svc.GetClientByToken(token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}

	// 闂傚倸鍊搁崐椋庣矆娓氣偓瀹曘儳鈧綆鍠栫壕鍧楁煙閹増顥夐幖鏉戯躬閺屻倝鎳濋幍顔肩墯婵炲瓨绮岀紞濠囧蓟濞戙垹唯闁瑰瓨绻冨﹢鐗堜繆濡炵厧濮傛慨濠冩そ楠炴劖鎯旈敐鍥╂殼婵犵數鍋犵亸娆愮箾閳ь剟鏌涢埞鎯т壕婵＄偑鍊栫敮濠勬閵堝鍤€闁秆勵殕閻?Panel URL
	panelURL := s.getPanelURL(c)

	script := fmt.Sprintf(`#!/bin/bash
# GOST 闂傚倸鍊峰ù鍥敋瑜庨〃銉╁传閵壯傜瑝閻庡箍鍎遍ˇ顖炲垂閸屾稓绠剧€瑰壊鍠曠花濠氭煛閸曗晛鍔滅紒缁樼洴楠炲鎮欑€靛憡顓婚梻浣告啞椤ㄥ棛鍠婂澶娢﹂柛鏇ㄥ灠閸愨偓闂侀潧顭俊鍥р枔閵堝鈷戦柛婵嗗椤ョ偞淇婇銏犳殻妤犵偛鍟撮弻銊р偓锝庡亜椤庢捇姊洪崨濠勨槈闁挎洏鍊曢埢鎾崇暆閸曨兘鎷洪梺鍛婄☉閿曘儳浜搁銏＄厪闁割偁鍩勯悞鐐亜?(Agent 濠电姷鏁告慨鐑姐€傞挊澹╋綁宕ㄩ弶鎴濈€銈呯箰閻楀棝鎮為崹顐犱簻闁瑰搫妫楁禍鍓х磼閸撗嗘闁告ɑ鍎抽埥澶愭偨缁嬭法鍔?
# 闂傚倸鍊峰ù鍥敋瑜庨〃銉╁传閵壯傜瑝閻庡箍鍎遍ˇ顖炲垂閸屾稓绠剧€瑰壊鍠曠花濠氭煛閸曗晛鍔滅紒缁樼洴楠炲鎮欑€靛憡顓婚梻? %s

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

REPO="supernaga/gostpanel"
PANEL_URL="%s"
CLIENT_TOKEN="%s"
INSTALL_DIR="/opt/gost-panel"

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

dl() {
    local url="$1" output="$2"
    if command -v curl &>/dev/null; then
        [ -n "$output" ] && curl -fsSL "$url" -o "$output" || curl -fsSL "$url"
    elif command -v wget &>/dev/null; then
        [ -n "$output" ] && wget -qO "$output" "$url" || wget -qO- "$url"
    else
        log_error "curl and wget not found"; exit 1
    fi
}

detect_arch() {
    local arch=$(uname -m)
    case $arch in
        x86_64|amd64) echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        armv7l|armv7) echo "armv7" ;;
        armv6l|armv6) echo "armv6" ;;
        armv5*) echo "armv5" ;;
        mips) echo "mipsle" ;;
        mips64) echo "mips64le" ;;
        i386|i686) echo "386" ;;
        *) log_error "Unsupported architecture: $arch"; exit 1 ;;
    esac
}

echo "======================================"
echo "  GOST Panel Client Installer"
echo "  (Agent Mode - Built-in Heartbeat)"
echo "======================================"
echo ""

GOST_ARCH=$(detect_arch)
log_info "Architecture: $GOST_ARCH"
log_info "Panel: $PANEL_URL"

# 濠电姷鏁告慨鐑藉极閹间礁纾婚柣鎰惈缁犱即鏌熼梻瀵割槮缂佺姷濞€閺岀喖鎮ч崼鐔哄嚒缂備胶濮甸悧鏇㈠煘閹达附鍋愰柛娆忣槹閹瑧绱撴笟鍥т簻缂佸鐖奸獮澶婎潰瀹€鈧悿鈧梺鍦檸閸ㄩ亶鎮?shell 闂傚倸鍊搁崐鎼佲€﹂鍕；闁告洦鍊嬪ú顏勎у璺猴功閺屽牓姊虹憴鍕姸濠殿喓鍊濆畷?(婵犵數濮烽弫鎼佸磻濞戙埄鏁嬫い鎾跺枑閸欏繘鎮楅悽鐢点€婇柛瀣尭閳藉骞掗幘瀵稿絽闂佹崘宕甸崑鐐哄Φ閸曨垰绠涢柛顐ｆ礃椤庡秹姊虹粙娆惧剰妞ゆ垵顦靛璇测槈閵忊晜鏅濋梺闈涚墕閹冲繘鎮楁ィ鍐┾拺闁革富鍘奸崢鏉懨瑰鍐煟鐎殿喛顕ч埥澶娢熷鍕棃闁糕斁鍋撳銈嗗坊閸嬫捇鎮￠妶澶嬬厪濠电姴绻愰惁婊勭箾?
if [ -f /etc/gost/heartbeat.sh ]; then
    log_info "Cleaning up old heartbeat..."
    systemctl stop gost-heartbeat.timer 2>/dev/null || true
    systemctl disable gost-heartbeat.timer 2>/dev/null || true
    rm -f /etc/systemd/system/gost-heartbeat.service
    rm -f /etc/systemd/system/gost-heartbeat.timer
    (crontab -l 2>/dev/null | grep -v "gost/heartbeat") | crontab - 2>/dev/null || true
    rm -f /etc/gost/heartbeat.sh
    systemctl stop gost 2>/dev/null || true
    systemctl disable gost 2>/dev/null || true
    rm -f /etc/systemd/system/gost.service
    systemctl daemon-reload 2>/dev/null || true
fi
if systemctl is-active gost-client &>/dev/null 2>&1; then
    systemctl stop gost-client 2>/dev/null || true
    systemctl disable gost-client 2>/dev/null || true
    rm -f /etc/systemd/system/gost-client.service
    systemctl daemon-reload 2>/dev/null || true
fi

# 婵犵數濮烽弫鎼佸磻閻愬搫鍨傞柛顐ｆ礀缁犱即鏌熼梻瀵歌窗闁轰礁瀚伴弻娑㈩敃閿濆洩绌?Agent
log_info "[1/3] Installing Agent..."
mkdir -p "$INSTALL_DIR"
rm -f "$INSTALL_DIR/gost-agent"

latest_version=$(dl "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
[ -z "$latest_version" ] && latest_version="v1.0.0"

agent_url="https://github.com/$REPO/releases/download/$latest_version/gost-agent-linux-$GOST_ARCH"
log_info "Downloading agent ($latest_version)..."

if dl "$agent_url" "$INSTALL_DIR/gost-agent" 2>/dev/null; then
    chmod +x "$INSTALL_DIR/gost-agent"
    log_info "Agent downloaded"
else
    log_warn "GitHub download failed, trying panel..."
    if dl "$PANEL_URL/agent/download/linux/$GOST_ARCH" "$INSTALL_DIR/gost-agent" 2>/dev/null; then
        chmod +x "$INSTALL_DIR/gost-agent"
    else
        log_error "Failed to download agent"; exit 1
    fi
fi

# 闂傚倸鍊峰ù鍥敋瑜嶉湁闁绘垼妫勭粻鐘绘煙閹冩闁搞儺鍓欑粻顕€鏌涢幘宕囦虎妞わ附澹嗛幑銏犫攽鐎ｎ亞鍊為悷婊冪Ч閹潧顫滈埀顒勫箖濡ゅ懐宓侀柛顭戝枛婵酣姊洪悷鏉挎毐闂佸府缍侀弫?
log_info "[2/3] Installing service..."
$INSTALL_DIR/gost-agent service install -panel $PANEL_URL -token $CLIENT_TOKEN -mode client
$INSTALL_DIR/gost-agent service start

# 闂傚倸鍊峰ù鍥敋瑜嶉湁闁绘垼妫勭粻鐘绘煙閹规劦鍤欓悗姘槹閵囧嫰骞掗幋婵愪患闂?
log_info "[3/3] Done!"
echo ""
echo "======================================"
echo "  Installation Complete!"
echo "======================================"
echo ""
echo "Agent Mode Features:"
echo "  - Built-in heartbeat (every 30s)"
echo "  - Auto config reload"
echo "  - Auto GOST download"
echo "  - Auto uninstall when deleted from panel"
echo ""
echo "Commands:"
echo "  $INSTALL_DIR/gost-agent service status   - Check status"
echo "  $INSTALL_DIR/gost-agent service restart  - Restart"
echo "  journalctl -u gost-client -f             - View logs"
`, client.Name, panelURL, client.Token)

	c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	c.String(http.StatusOK, script)
}
