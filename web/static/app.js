let autoRefreshInterval = null;

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

    // Convert datetime-local format to RFC3339
    if (startTime) {
        const startDate = new Date(startTime);
        params.append('start', startDate.toISOString());
    }
    if (endTime) {
        const endDate = new Date(endTime);
        params.append('end', endDate.toISOString());
    }

    try {
        const response = await fetch(`/api/logs?${params}`);
        const logs = await response.json();

        displayLogs(logs);
    } catch (error) {
        console.error('Failed to load logs:', error);
        document.getElementById('logsContainer').innerHTML =
            '<div class="loading">Error loading logs</div>';
    }
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
                    <div class="detail-value">${new Date(log.created_at).toLocaleString()}</div>
                </div>` : ''}
            </div>
        `;

        return `
            <div class="log-entry ${levelClass}" data-index="${index}">
                <div class="log-header">
                    <span class="log-timestamp">${timestamp}</span>
                    <span class="log-service">${escapeHtml(log.service)}</span>
                    <span class="log-level ${levelClass}">${escapeHtml(log.level)}</span>
                    <span class="log-host">${escapeHtml(log.host || '')}</span>
                </div>
                <div class="log-message">${escapeHtml(log.message)}</div>
                ${detailsHtml}
                ${metadataHtml}
            </div>
        `;
    }).join('');

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
    document.getElementById(id).addEventListener('change', loadLogs);
});

// Search with debounce
let searchTimeout;
document.getElementById('search').addEventListener('input', () => {
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(loadLogs, 500);
});

// Initial load
loadFilterOptions();
loadLogs();
