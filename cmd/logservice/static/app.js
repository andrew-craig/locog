let autoRefreshInterval = null;
let ws = null;
let wsReconnectTimeout = null;
let currentLogs = [];

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
    const limit = document.getElementById('limit').value;
    const startTime = document.getElementById('startTime').value;
    const endTime = document.getElementById('endTime').value;

    if (service) params.append('service', service);
    if (level) params.append('level', level);
    if (host) params.append('host', host);
    if (search) params.append('search', search);
    if (limit) params.append('limit', limit);

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

// Auto-refresh toggle
document.getElementById('autoRefresh').addEventListener('change', (e) => {
    if (e.target.checked) {
        loadLogs(); // Load immediately
        autoRefreshInterval = setInterval(loadLogs, 10000); // Then every 10s
    } else {
        if (autoRefreshInterval) {
            clearInterval(autoRefreshInterval);
            autoRefreshInterval = null;
        }
    }
});

// Load logs when filters change
['service', 'level', 'host', 'limit', 'startTime', 'endTime'].forEach(id => {
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

            // Respect the limit setting
            const limit = parseInt(document.getElementById('limit').value) || 500;
            if (currentLogs.length > limit) {
                currentLogs = currentLogs.slice(0, limit);
            }

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
loadFilterOptions();
loadLogs();
connectWebSocket();
