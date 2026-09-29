package download

import (
	"strings"

	"typhon/internal/uierr"
)

const (
	eventNetwork = "download:network"

	NetworkOK   = "ok"
	NetworkDown = "down"
)

var (
	errNetworkDown       = uierr.New("download.network_down", "сеть недоступна: VPN или прокси не отвечает, загрузки остановлены")
	errNetIfaceMissing   = uierr.New("download.net_interface_missing", "сетевой адаптер не найден")
	errNetIfaceDown      = uierr.New("download.net_interface_down", "сетевой адаптер отключён")
	errNetIfaceNoAddr    = uierr.New("download.net_interface_no_address", "у сетевого адаптера нет подходящего IP-адреса")
	errNetIfaceList      = uierr.New("download.net_interface_list_failed", "не удалось получить список сетевых адаптеров")
	errNetIfaceWeakHost  = uierr.New("download.net_interface_weak_host", "на адаптере включена отправка с чужого интерфейса (WeakHostSend): трафик мог бы пойти мимо VPN")
	errNetIfaceHostCheck = uierr.New("download.net_interface_host_check_failed", "не удалось проверить настройки адаптера")
	errNetIfaceUnsupport = uierr.New("download.net_interface_unsupported", "привязка к адаптеру поддерживается только в Windows")
	errNetIfaceNoDNS     = uierr.New("download.net_interface_no_dns", "у адаптера нет DNS-серверов: трекеры и DHT-узлы по именам не используются")
	errProxyUnreachable  = uierr.New("download.proxy_unreachable", "прокси недоступен")
	errProxyAuthFailed   = uierr.New("download.proxy_auth_failed", "прокси отклонил логин или пароль")
	errProxyFailed       = uierr.New("download.proxy_failed", "прокси ответил ошибкой")
	errProxyCredentials  = uierr.New("download.proxy_credentials_failed", "не удалось прочитать пароль прокси из хранилища учётных данных")
	errProxyMismatch     = uierr.New("download.proxy_credentials_mismatch", "сохранённый пароль относится к другому имени пользователя прокси: введите пароль заново")
	errProxyNotSet       = uierr.New("download.proxy_not_configured", "прокси не настроен")
	errProxyPasswordSize = uierr.New("download.proxy_password_invalid", "пароль прокси не должен быть длиннее 255 байт")
	errNoMetadataProxy   = uierr.New("download.no_metadata_proxy", "метаданные не получены: через прокси работают только HTTP-трекеры, поиск по DHT и UDP-трекерам отключён")
)

// NetworkState describes where torrent traffic is allowed to go right now.
// State is "ok" while the tunnel or proxy the user asked for is in use and
// "down" while it is not; Code is then the ui error code of the reason.
// Warning is a ui error code too: the network works, but something the user
// would expect to work does not.
type NetworkState struct {
	Mode    string `json:"mode"`
	State   string `json:"state"`
	Code    string `json:"code"`
	Reason  string `json:"reason"`
	Address string `json:"address"`
	Warning string `json:"warning"`
}

type NetInterface struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Addresses   []string `json:"addresses"`
	Up          bool     `json:"up"`
	VPNLike     bool     `json:"vpnLike"`
}

func reasonOf(err error) (code, reason string) {
	code = uierr.Code(err)
	reason = err.Error()
	if code != "" {
		reason = strings.Replace(reason, "typhon:"+code+": ", "", 1)
	}
	return code, reason
}
