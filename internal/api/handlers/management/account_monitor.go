package management

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// AccountStatus represents the status of a single auth account for monitoring.
type AccountStatus struct {
	ID            string                 `json:"id"`
	Provider      string                 `json:"provider"`
	Label         string                 `json:"label"`
	Email         string                 `json:"email,omitempty"`
	Status        string                 `json:"status"`
	StatusMessage string                 `json:"status_message,omitempty"`
	Disabled      bool                   `json:"disabled"`
	Unavailable   bool                   `json:"unavailable"`
	QuotaExceeded bool                   `json:"quota_exceeded"`
	QuotaReason   string                 `json:"quota_reason,omitempty"`
	NextRecoverAt *time.Time             `json:"next_recover_at,omitempty"`
	NextRetryAt   *time.Time             `json:"next_retry_at,omitempty"`
	BackoffLevel  int                    `json:"backoff_level"`
	LastError     map[string]interface{} `json:"last_error,omitempty"`
	LastRefresh   *time.Time             `json:"last_refresh,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	Index         uint64                 `json:"index"`
}

// AccountsMonitorResponse is the response structure for the accounts monitor endpoint.
type AccountsMonitorResponse struct {
	Timestamp    time.Time       `json:"timestamp"`
	TotalCount   int             `json:"total_count"`
	ActiveCount  int             `json:"active_count"`
	ErrorCount   int             `json:"error_count"`
	CooldownCount int            `json:"cooldown_count"`
	Accounts     []AccountStatus `json:"accounts"`
}

// GetAccountsMonitor returns detailed status of all auth accounts for monitoring.
func (h *Handler) GetAccountsMonitor(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "handler not initialized"})
		return
	}

	if h.authManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth manager not available"})
		return
	}

	auths := h.authManager.List()
	now := time.Now()

	response := AccountsMonitorResponse{
		Timestamp: now,
		Accounts:  make([]AccountStatus, 0, len(auths)),
	}

	for _, auth := range auths {
		if auth == nil {
			continue
		}

		status := AccountStatus{
			ID:            auth.ID,
			Provider:      auth.Provider,
			Label:         auth.Label,
			Status:        string(auth.Status),
			StatusMessage: auth.StatusMessage,
			Disabled:      auth.Disabled,
			Unavailable:   auth.Unavailable,
			QuotaExceeded: auth.Quota.Exceeded,
			QuotaReason:   auth.Quota.Reason,
			BackoffLevel:  auth.Quota.BackoffLevel,
			CreatedAt:     auth.CreatedAt,
			UpdatedAt:     auth.UpdatedAt,
			Index:         auth.Index,
		}

		// Extract email from metadata
		if auth.Metadata != nil {
			if email, ok := auth.Metadata["email"].(string); ok {
				status.Email = email
			}
		}

		// Set recovery times if applicable
		if !auth.Quota.NextRecoverAt.IsZero() {
			t := auth.Quota.NextRecoverAt
			status.NextRecoverAt = &t
		}
		if !auth.NextRetryAfter.IsZero() {
			t := auth.NextRetryAfter
			status.NextRetryAt = &t
		}

		// Extract last refresh from metadata
		if ts, ok := extractLastRefreshTimestamp(auth.Metadata); ok {
			status.LastRefresh = &ts
		}

		// Copy last error if present
		if auth.LastError != nil {
			status.LastError = map[string]interface{}{
				"code":    auth.LastError.Code,
				"message": auth.LastError.Message,
			}
			if auth.LastError.HTTPStatus != 0 {
				status.LastError["http_status"] = auth.LastError.HTTPStatus
			}
		}

		response.Accounts = append(response.Accounts, status)

		// Count statistics
		response.TotalCount++
		if auth.Disabled {
			continue
		}
		if auth.Quota.Exceeded || (auth.Unavailable && !auth.Quota.NextRecoverAt.IsZero() && auth.Quota.NextRecoverAt.After(now)) {
			response.CooldownCount++
		} else if auth.Unavailable || auth.Status == "error" {
			response.ErrorCount++
		} else {
			response.ActiveCount++
		}
	}

	c.JSON(http.StatusOK, response)
}

// ServeAccountMonitorPage serves the account monitor HTML page.
func (h *Handler) ServeAccountMonitorPage(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, accountMonitorHTML)
}

const accountMonitorHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Account Monitor - CLIProxyAPI</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            background: #0d1117;
            color: #c9d1d9;
            min-height: 100vh;
            padding: 20px;
        }
        .container { max-width: 1400px; margin: 0 auto; }
        h1 { color: #58a6ff; margin-bottom: 20px; font-size: 24px; }
        .header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
            flex-wrap: wrap;
            gap: 10px;
        }
        .stats {
            display: flex;
            gap: 15px;
            flex-wrap: wrap;
        }
        .stat-card {
            background: #161b22;
            border: 1px solid #30363d;
            border-radius: 8px;
            padding: 15px 20px;
            min-width: 120px;
        }
        .stat-card .label { font-size: 12px; color: #8b949e; margin-bottom: 5px; }
        .stat-card .value { font-size: 28px; font-weight: 600; }
        .stat-card.active .value { color: #3fb950; }
        .stat-card.error .value { color: #f85149; }
        .stat-card.cooldown .value { color: #d29922; }
        .stat-card.total .value { color: #58a6ff; }
        .controls {
            display: flex;
            gap: 10px;
            align-items: center;
            flex-wrap: wrap;
        }
        input, button, select {
            background: #21262d;
            border: 1px solid #30363d;
            color: #c9d1d9;
            padding: 8px 12px;
            border-radius: 6px;
            font-size: 14px;
        }
        input:focus, select:focus { outline: none; border-color: #58a6ff; }
        button {
            background: #238636;
            border-color: #238636;
            cursor: pointer;
            font-weight: 500;
        }
        button:hover { background: #2ea043; }
        button.secondary { background: #21262d; border-color: #30363d; }
        button.secondary:hover { background: #30363d; }
        .filter-group { display: flex; gap: 5px; align-items: center; }
        .filter-group label { font-size: 12px; color: #8b949e; }
        .accounts-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
            gap: 15px;
            margin-top: 20px;
        }
        .account-card {
            background: #161b22;
            border: 1px solid #30363d;
            border-radius: 8px;
            padding: 15px;
            transition: border-color 0.2s;
        }
        .account-card:hover { border-color: #58a6ff; }
        .account-card.status-active { border-left: 3px solid #3fb950; }
        .account-card.status-error { border-left: 3px solid #f85149; }
        .account-card.status-cooldown { border-left: 3px solid #d29922; }
        .account-card.status-disabled { border-left: 3px solid #484f58; opacity: 0.6; }
        .account-header {
            display: flex;
            justify-content: space-between;
            align-items: flex-start;
            margin-bottom: 10px;
        }
        .account-provider {
            background: #30363d;
            padding: 2px 8px;
            border-radius: 12px;
            font-size: 11px;
            text-transform: uppercase;
            font-weight: 600;
        }
        .account-provider.codex { background: #238636; }
        .account-provider.claude { background: #8957e5; }
        .account-provider.gemini { background: #1a73e8; }
        .account-provider.gemini-cli { background: #4285f4; }
        .account-email {
            font-size: 14px;
            font-weight: 500;
            color: #f0f6fc;
            word-break: break-all;
        }
        .account-id {
            font-size: 11px;
            color: #8b949e;
            font-family: monospace;
            margin-top: 2px;
        }
        .account-status {
            display: flex;
            align-items: center;
            gap: 6px;
            margin: 10px 0;
        }
        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
        }
        .status-dot.active { background: #3fb950; }
        .status-dot.error { background: #f85149; }
        .status-dot.cooldown { background: #d29922; animation: pulse 2s infinite; }
        .status-dot.disabled { background: #484f58; }
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
        .status-text { font-size: 13px; }
        .account-details {
            font-size: 12px;
            color: #8b949e;
            margin-top: 10px;
            padding-top: 10px;
            border-top: 1px solid #21262d;
        }
        .detail-row {
            display: flex;
            justify-content: space-between;
            margin-bottom: 4px;
        }
        .detail-row .label { color: #8b949e; }
        .detail-row .value { color: #c9d1d9; font-family: monospace; }
        .detail-row .value.error { color: #f85149; }
        .detail-row .value.warning { color: #d29922; }
        .detail-row .value.success { color: #3fb950; }
        .error-message {
            background: #f8514915;
            border: 1px solid #f8514930;
            border-radius: 4px;
            padding: 8px;
            margin-top: 8px;
            font-size: 11px;
            color: #f85149;
        }
        .countdown {
            font-family: monospace;
            color: #d29922;
        }
        .refresh-indicator {
            display: inline-block;
            width: 12px;
            height: 12px;
            border: 2px solid #30363d;
            border-top-color: #58a6ff;
            border-radius: 50%;
            animation: spin 1s linear infinite;
            margin-left: 8px;
        }
        .refresh-indicator.hidden { display: none; }
        @keyframes spin { to { transform: rotate(360deg); } }
        .empty-state {
            text-align: center;
            padding: 60px 20px;
            color: #8b949e;
        }
        .empty-state h2 { color: #c9d1d9; margin-bottom: 10px; }
        .last-update { font-size: 12px; color: #8b949e; }
        .toast {
            position: fixed;
            bottom: 20px;
            right: 20px;
            background: #161b22;
            border: 1px solid #30363d;
            padding: 12px 20px;
            border-radius: 8px;
            display: none;
        }
        .toast.show { display: block; }
        .toast.error { border-color: #f85149; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div>
                <h1>Account Monitor <span class="refresh-indicator hidden" id="refreshIndicator"></span></h1>
                <div class="last-update">Last updated: <span id="lastUpdate">-</span></div>
            </div>
            <div class="controls">
                <div class="filter-group">
                    <label>Provider:</label>
                    <select id="providerFilter">
                        <option value="">All</option>
                        <option value="codex">Codex</option>
                        <option value="claude">Claude</option>
                        <option value="gemini-cli">Gemini CLI</option>
                        <option value="gemini">Gemini</option>
                    </select>
                </div>
                <div class="filter-group">
                    <label>Status:</label>
                    <select id="statusFilter">
                        <option value="">All</option>
                        <option value="active">Active</option>
                        <option value="cooldown">Cooldown</option>
                        <option value="error">Error</option>
                        <option value="disabled">Disabled</option>
                    </select>
                </div>
                <button onclick="refreshData()" class="secondary">Refresh</button>
                <div class="filter-group">
                    <label>Auto:</label>
                    <select id="autoRefresh">
                        <option value="0">Off</option>
                        <option value="5">5s</option>
                        <option value="10" selected>10s</option>
                        <option value="30">30s</option>
                        <option value="60">60s</option>
                    </select>
                </div>
            </div>
        </div>
        <div class="stats" id="statsContainer">
            <div class="stat-card total"><div class="label">Total</div><div class="value" id="statTotal">-</div></div>
            <div class="stat-card active"><div class="label">Active</div><div class="value" id="statActive">-</div></div>
            <div class="stat-card cooldown"><div class="label">Cooldown</div><div class="value" id="statCooldown">-</div></div>
            <div class="stat-card error"><div class="label">Error</div><div class="value" id="statError">-</div></div>
        </div>
        <div class="accounts-grid" id="accountsGrid"></div>
    </div>
    <div class="toast" id="toast"></div>

    <script>
        let accounts = [];
        let autoRefreshInterval = null;
        const API_KEY = localStorage.getItem('management_key') || '';

        async function fetchAccounts() {
            const indicator = document.getElementById('refreshIndicator');
            indicator.classList.remove('hidden');
            try {
                const headers = { 'Content-Type': 'application/json' };
                if (API_KEY) headers['Authorization'] = 'Bearer ' + API_KEY;
                const resp = await fetch('/v0/management/accounts-monitor', { headers });
                if (!resp.ok) {
                    if (resp.status === 401 || resp.status === 403) {
                        const key = prompt('Enter management key:');
                        if (key) {
                            localStorage.setItem('management_key', key);
                            location.reload();
                        }
                        return null;
                    }
                    throw new Error('HTTP ' + resp.status);
                }
                return await resp.json();
            } catch (e) {
                showToast('Failed to fetch accounts: ' + e.message, true);
                return null;
            } finally {
                indicator.classList.add('hidden');
            }
        }

        function updateStats(data) {
            document.getElementById('statTotal').textContent = data.total_count;
            document.getElementById('statActive').textContent = data.active_count;
            document.getElementById('statCooldown').textContent = data.cooldown_count;
            document.getElementById('statError').textContent = data.error_count;
            document.getElementById('lastUpdate').textContent = new Date(data.timestamp).toLocaleTimeString();
        }

        function getAccountStatus(account) {
            if (account.disabled) return 'disabled';
            if (account.quota_exceeded) return 'cooldown';
            if (account.unavailable && account.next_recover_at) return 'cooldown';
            if (account.unavailable || account.status === 'error') return 'error';
            return 'active';
        }

        function formatDuration(ms) {
            if (ms <= 0) return 'now';
            const s = Math.floor(ms / 1000);
            const m = Math.floor(s / 60);
            const h = Math.floor(m / 60);
            if (h > 0) return h + 'h ' + (m % 60) + 'm';
            if (m > 0) return m + 'm ' + (s % 60) + 's';
            return s + 's';
        }

        function renderAccounts(data) {
            const grid = document.getElementById('accountsGrid');
            const providerFilter = document.getElementById('providerFilter').value.toLowerCase();
            const statusFilter = document.getElementById('statusFilter').value;

            let filtered = data.accounts.filter(a => {
                if (providerFilter && !a.provider.toLowerCase().includes(providerFilter)) return false;
                if (statusFilter && getAccountStatus(a) !== statusFilter) return false;
                return true;
            });

            if (filtered.length === 0) {
                grid.innerHTML = '<div class="empty-state"><h2>No accounts found</h2><p>No accounts match the current filters</p></div>';
                return;
            }

            grid.innerHTML = filtered.map(account => {
                const status = getAccountStatus(account);
                const now = Date.now();
                let recoveryTime = '';
                if (account.next_recover_at) {
                    const recoverAt = new Date(account.next_recover_at).getTime();
                    if (recoverAt > now) {
                        recoveryTime = formatDuration(recoverAt - now);
                    }
                }

                let errorHtml = '';
                if (account.last_error && account.last_error.message) {
                    errorHtml = '<div class="error-message">' + escapeHtml(account.last_error.message) + '</div>';
                }

                return '<div class="account-card status-' + status + '">' +
                    '<div class="account-header">' +
                        '<div>' +
                            '<div class="account-email">' + escapeHtml(account.email || account.label || 'Unknown') + '</div>' +
                            '<div class="account-id">#' + account.index + ' • ' + escapeHtml(account.id.substring(0, 20)) + '...</div>' +
                        '</div>' +
                        '<span class="account-provider ' + account.provider + '">' + escapeHtml(account.provider) + '</span>' +
                    '</div>' +
                    '<div class="account-status">' +
                        '<span class="status-dot ' + status + '"></span>' +
                        '<span class="status-text">' + getStatusText(account, status, recoveryTime) + '</span>' +
                    '</div>' +
                    '<div class="account-details">' +
                        (account.quota_reason ? '<div class="detail-row"><span class="label">Quota Reason</span><span class="value warning">' + escapeHtml(account.quota_reason) + '</span></div>' : '') +
                        (recoveryTime ? '<div class="detail-row"><span class="label">Recovery In</span><span class="value countdown">' + recoveryTime + '</span></div>' : '') +
                        (account.backoff_level > 0 ? '<div class="detail-row"><span class="label">Backoff Level</span><span class="value">' + account.backoff_level + '</span></div>' : '') +
                        '<div class="detail-row"><span class="label">Last Refresh</span><span class="value">' + (account.last_refresh ? new Date(account.last_refresh).toLocaleString() : '-') + '</span></div>' +
                        '<div class="detail-row"><span class="label">Updated</span><span class="value">' + new Date(account.updated_at).toLocaleString() + '</span></div>' +
                    '</div>' +
                    errorHtml +
                '</div>';
            }).join('');
        }

        function getStatusText(account, status, recoveryTime) {
            if (status === 'disabled') return 'Disabled';
            if (status === 'cooldown') return 'Cooldown' + (recoveryTime ? ' (' + recoveryTime + ')' : '');
            if (status === 'error') return account.status_message || 'Error';
            return 'Active';
        }

        function escapeHtml(str) {
            if (!str) return '';
            return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        }

        function showToast(message, isError) {
            const toast = document.getElementById('toast');
            toast.textContent = message;
            toast.className = 'toast show' + (isError ? ' error' : '');
            setTimeout(() => toast.classList.remove('show'), 3000);
        }

        async function refreshData() {
            const data = await fetchAccounts();
            if (data) {
                accounts = data.accounts;
                updateStats(data);
                renderAccounts(data);
            }
        }

        function setupAutoRefresh() {
            if (autoRefreshInterval) clearInterval(autoRefreshInterval);
            const seconds = parseInt(document.getElementById('autoRefresh').value);
            if (seconds > 0) {
                autoRefreshInterval = setInterval(refreshData, seconds * 1000);
            }
        }

        document.getElementById('autoRefresh').addEventListener('change', setupAutoRefresh);
        document.getElementById('providerFilter').addEventListener('change', () => renderAccounts({ accounts }));
        document.getElementById('statusFilter').addEventListener('change', () => renderAccounts({ accounts }));

        // Initial load
        refreshData();
        setupAutoRefresh();
    </script>
</body>
</html>`
