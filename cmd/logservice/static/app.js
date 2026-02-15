let ws = null;
let wsReconnectTimeout = null;
let currentLogs = [];

// Theme management
function initTheme() {
    const savedTheme = localStorage.getItem('theme') || 'auto';
    applyTheme(savedTheme);
    updateThemeMenuSelection(savedTheme);
}

function setTheme(theme) {
    localStorage.setItem('theme', theme);
    applyTheme(theme);
    updateThemeMenuSelection(theme);
    toggleThemeMenu();
}

function applyTheme(theme) {
    const html = document.documentElement;

    if (theme === 'light') {
        html.setAttribute('data-theme', 'light');
    } else if (theme === 'dark') {
        html.setAttribute('data-theme', 'dark');
    } else {
        // auto mode - remove attribute to use prefers-color-scheme
        html.removeAttribute('data-theme');
    }

    updateThemeIcon(theme);
}

function updateThemeIcon(theme) {
    const themeIcon = document.getElementById('themeIcon');
    if (!themeIcon) return;

    if (theme === 'light') {
        // Sun icon
        themeIcon.innerHTML = '<circle cx="12" cy="12" r="5"></circle><line x1="12" y1="1" x2="12" y2="3"></line><line x1="12" y1="21" x2="12" y2="23"></line><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line><line x1="1" y1="12" x2="3" y2="12"></line><line x1="21" y1="12" x2="23" y2="12"></line><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line>';
    } else if (theme === 'dark') {
        // Moon icon
        themeIcon.innerHTML = '<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path>';
    } else {
        // Monitor/auto icon
        themeIcon.innerHTML = '<rect x="2" y="3" width="20" height="14" rx="2" ry="2"></rect><line x1="8" y1="21" x2="16" y2="21"></line><line x1="12" y1="17" x2="12" y2="21"></line>';
    }
}

function updateThemeMenuSelection(theme) {
    const options = document.querySelectorAll('.theme-option');
    options.forEach(option => {
        option.classList.remove('active');
    });

    const themeMap = { 'light': 0, 'dark': 1, 'auto': 2 };
    const index = themeMap[theme] || 2;
    if (options[index]) {
        options[index].classList.add('active');
    }
}

function toggleThemeMenu() {
    const menu = document.getElementById('themeMenu');
    menu.classList.toggle('show');
}

// Close theme menu when clicking outside
document.addEventListener('click', function(e) {
    const themeSwitcher = document.querySelector('.theme-switcher');
    const themeMenu = document.getElementById('themeMenu');

    if (themeSwitcher && !themeSwitcher.contains(e.target)) {
        themeMenu.classList.remove('show');
    }
});

function toggleMobileFilters() {
    const filters = document.querySelector('.filters');
    filters.classList.toggle('mobile-expanded');
}

function updateMobileFilterSummary() {
    const summary = document.getElementById('mobileFilterSummary');
    if (!summary) return;

    const tags = [];
    const level = document.getElementById('level').value;
    const host = document.getElementById('host').value;
    const service = document.getElementById('service').value;
    const search = document.getElementById('search').value;
    const startTime = document.getElementById('startTime').value;
    const endTime = document.getElementById('endTime').value;

    if (level) tags.push('Level: ' + level);
    if (host) tags.push('Host: ' + host);
    if (service) tags.push('Service: ' + service);
    if (search) tags.push('Search: ' + search);
    if (startTime || endTime) {
        const parts = [];
        if (startTime) parts.push(startTime);
        if (endTime) parts.push(endTime);
        tags.push('Dates: ' + parts.join(' - '));
    }

    if (tags.length === 0) {
        summary.textContent = 'No filters applied';
    } else {
        summary.innerHTML = tags.map(t => '<span class="filter-tag">' + escapeHtml(t) + '</span>').join('');
    }
}

// Load filter options on page load
async function loadFilterOptions() {
    try {
        const response = await fetch('/api/filters');
        const options = await response.json();

        populateSelect('service', options.services);
        populateSelect('level', options.levels);
        populateSelect('host', options.hosts);
    } catch (error) {
        console.error('Failed to load filter options:', error);
    }
}

function populateSelect(id, values) {
    const select = document.getElementById(id);
    const currentValue = select.value;

    // Keep "All" option and add values
    select.innerHTML = '<option value="">All</option>';
    if (values) {
        values.forEach(value => {
            const option = document.createElement('option');
            option.value = value;
            option.textContent = value;
            select.appendChild(option);
        });
    }

    // Restore previous selection if it still exists
    if (values && values.includes(currentValue)) {
        select.value = currentValue;
    }
}

async function loadLogs() {
    const params = new URLSearchParams();

    const service = document.getElementById('service').value;
    const level = document.getElementById('level').value;
    const host = document.getElementById('host').value;
    const search = document.getElementById('search').value;
    const startTime = document.getElementById('startTime').value;
    const endTime = document.getElementById('endTime').value;

    if (service) params.append('service', service);
    if (level) params.append('level', level);
    if (host) params.append('host', host);
    if (search) params.append('search', search);

    // Convert date format to RFC3339
    if (startTime) {
        const startDate = new Date(startTime + 'T00:00:00Z');
        params.append('start', startDate.toISOString());
    }
    if (endTime) {
        const endDate = new Date(endTime + 'T23:59:59.999Z');
        params.append('end', endDate.toISOString());
    }

    try {
        const response = await fetch(`/api/logs?${params}`);
        const logs = await response.json();

        currentLogs = logs || [];
        displayLogs(currentLogs);
    } catch (error) {
        console.error('Failed to load logs:', error);
        document.getElementById('logsContainer').innerHTML =
            '<div class="loading">Error loading logs</div>';
    }
}

function getLogLevelIcon(level) {
    const iconMap = {
        'ERROR': 'x-octagon',
        'WARN': 'alert-triangle',
        'INFO': 'alert-circle',
        'DEBUG': 'alert-circle'
    };
    return iconMap[level.toUpperCase()] || 'alert-circle';
}

function displayLogs(logs) {
    const container = document.getElementById('logsContainer');

    if (!logs || logs.length === 0) {
        container.innerHTML = '<div class="loading">No logs found</div>';
        return;
    }

    container.innerHTML = logs.map((log, index) => {
        const timestamp = new Date(log.timestamp).toLocaleString();
        const levelClass = escapeHtml(log.level.toLowerCase());
        const iconName = getLogLevelIcon(log.level);

        let metadataHtml = '';
        if (log.metadata && Object.keys(log.metadata).length > 0) {
            metadataHtml = `
                <div class="log-metadata">
                    ${escapeHtml(JSON.stringify(log.metadata, null, 2))}
                </div>
            `;
        }

        // Build details view with all log fields as key:value pairs
        const detailsHtml = `
            <div class="log-details">
                ${log.id ? `<div class="detail-row">
                    <div class="detail-key">ID:</div>
                    <div class="detail-value">${escapeHtml(String(log.id))}</div>
                </div>` : ''}
                <div class="detail-row">
                    <div class="detail-key">Timestamp:</div>
                    <div class="detail-value">${timestamp}</div>
                </div>
                <div class="detail-row">
                    <div class="detail-key">Service:</div>
                    <div class="detail-value">${escapeHtml(log.service)}</div>
                </div>
                <div class="detail-row">
                    <div class="detail-key">Level:</div>
                    <div class="detail-value">${escapeHtml(log.level)}</div>
                </div>
                ${log.host ? `<div class="detail-row">
                    <div class="detail-key">Host:</div>
                    <div class="detail-value">${escapeHtml(log.host)}</div>
                </div>` : ''}
                <div class="detail-row">
                    <div class="detail-key">Message:</div>
                    <div class="detail-value">${escapeHtml(log.message)}</div>
                </div>
                ${log.metadata && Object.keys(log.metadata).length > 0 ? `<div class="detail-row">
                    <div class="detail-key">Metadata:</div>
                    <div class="detail-value metadata">${escapeHtml(JSON.stringify(log.metadata, null, 2))}</div>
                </div>` : ''}
                ${log.created_at ? `<div class="detail-row">
                    <div class="detail-key">Created At:</div>
                    <div class="detail-value">${new Date(log.created_at).toISOString()}</div>
                </div>` : ''}
            </div>
        `;

        return `
            <div class="log-entry ${levelClass}" data-index="${index}">
                <div class="log-header">
                    <i data-feather="${iconName}" class="log-level-icon ${escapeHtml(log.level)}"></i>
                    <span class="log-timestamp">${timestamp}</span>
                    <span class="log-service">${escapeHtml(log.service)}</span>
                    <span class="log-host">${escapeHtml(log.host || '')}</span>
                </div>
                <div class="log-message">${escapeHtml(log.message)}</div>
                ${detailsHtml}
                ${metadataHtml}
            </div>
        `;
    }).join('');

    // Replace feather icon placeholders with SVG
    feather.replace();

    // Add click handlers to toggle expansion
    attachLogClickHandlers();
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function attachLogClickHandlers() {
    const logEntries = document.querySelectorAll('.log-entry');
    logEntries.forEach(entry => {
        entry.addEventListener('click', function() {
            this.classList.toggle('expanded');
        });
    });
}

// Load logs when filters change
['service', 'level', 'host', 'startTime', 'endTime'].forEach(id => {
    document.getElementById(id).addEventListener('change', () => {
        updateMobileFilterSummary();
        loadLogs();
    });
});

// Search with debounce
let searchTimeout;
document.getElementById('search').addEventListener('input', () => {
    clearTimeout(searchTimeout);
    updateMobileFilterSummary();
    searchTimeout = setTimeout(loadLogs, 500);
});

// WebSocket for real-time log streaming
function connectWebSocket() {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
        return;
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = protocol + '//' + window.location.host + '/api/ws';

    ws = new WebSocket(wsUrl);

    ws.onopen = function() {
        console.log('WebSocket connected');
        updateWsStatus(true);
    };

    ws.onmessage = function(event) {
        try {
            const newLogs = JSON.parse(event.data);
            if (!Array.isArray(newLogs) || newLogs.length === 0) return;

            // Check if any new logs match current filters
            const matchingLogs = newLogs.filter(matchesCurrentFilters);
            if (matchingLogs.length === 0) return;

            // Prepend new logs (they appear newest-first)
            currentLogs = matchingLogs.concat(currentLogs);

            displayLogs(currentLogs);
        } catch (e) {
            console.error('Failed to parse WebSocket message:', e);
        }
    };

    ws.onclose = function() {
        console.log('WebSocket disconnected, reconnecting...');
        updateWsStatus(false);
        ws = null;
        // Reconnect after a delay
        wsReconnectTimeout = setTimeout(connectWebSocket, 3000);
    };

    ws.onerror = function(err) {
        console.error('WebSocket error:', err);
        ws.close();
    };
}

function matchesCurrentFilters(log) {
    const service = document.getElementById('service').value;
    const level = document.getElementById('level').value;
    const host = document.getElementById('host').value;
    const search = document.getElementById('search').value;
    const startTime = document.getElementById('startTime').value;
    const endTime = document.getElementById('endTime').value;

    if (service && log.service !== service) return false;
    if (level && log.level.toLowerCase() !== level.toLowerCase()) return false;
    if (host && log.host !== host) return false;
    if (search && !log.message.toLowerCase().includes(search.toLowerCase())) return false;

    if (startTime) {
        const start = new Date(startTime + 'T00:00:00Z');
        if (new Date(log.timestamp) < start) return false;
    }
    if (endTime) {
        const end = new Date(endTime + 'T23:59:59.999Z');
        if (new Date(log.timestamp) > end) return false;
    }

    return true;
}

function updateWsStatus(connected) {
    const indicator = document.getElementById('wsStatus');
    if (!indicator) return;
    indicator.className = 'ws-status ' + (connected ? 'connected' : 'disconnected');
    indicator.title = connected ? 'WebSocket connected - receiving real-time updates' : 'WebSocket disconnected - reconnecting...';
}

// Initial load
initTheme();
loadFilterOptions();
loadLogs();
connectWebSocket();
