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

    if (service) params.append('service', service);
    if (level) params.append('level', level);
    if (host) params.append('host', host);
    if (search) params.append('search', search);
    if (limit) params.append('limit', limit);

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

    container.innerHTML = logs.map(log => {
        const timestamp = new Date(log.timestamp).toLocaleString();
        const levelClass = log.level.toLowerCase();

        let metadataHtml = '';
        if (log.metadata && Object.keys(log.metadata).length > 0) {
            metadataHtml = `
                <div class="log-metadata">
                    ${JSON.stringify(log.metadata, null, 2)}
                </div>
            `;
        }

        return `
            <div class="log-entry ${levelClass}">
                <div class="log-header">
                    <span class="log-timestamp">${timestamp}</span>
                    <span class="log-service">${log.service}</span>
                    <span class="log-level ${log.level}">${log.level}</span>
                    <span class="log-host">${log.host}</span>
                </div>
                <div class="log-message">${escapeHtml(log.message)}</div>
                ${metadataHtml}
            </div>
        `;
    }).join('');
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
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
['service', 'level', 'host', 'limit'].forEach(id => {
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
