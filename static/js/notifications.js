class NotificationManager {
    constructor() {
        this.dropdown = null;
        this.badge = null;
        this.isOpen = false;
        this.notifications = [];
        this.unreadCount = 0;
        this.refreshInterval = null;

        this.init();
    }

    init() {
        this.createNotificationButton();
        this.createDropdown();
        this.attachEventListeners();
        this.startRefreshInterval();
        this.loadNotifications();
    }

    createNotificationButton() {
        // Find the authentication section in header
        const authSection = document.querySelector('.authentication');
        if (!authSection) return;

        // Create notification button
        const notificationBtn = document.createElement('div');
        notificationBtn.className = 'notification-button';
        notificationBtn.innerHTML = `
            <button class="notification-bell" aria-label="Notifications">
                <div class="notification-badge" data-count="0">
                    <i class="fa-solid fa-bell"></i>
                </div>
            </button>
        `;

        // Insert before user info or login links
        authSection.insertBefore(notificationBtn, authSection.firstChild);

        this.badge = notificationBtn.querySelector('.notification-badge');
        this.button = notificationBtn.querySelector('.notification-bell');
    }

    createDropdown() {
        if (!this.button) return;

        const dropdown = document.createElement('div');
        dropdown.className = 'notification-dropdown';
        dropdown.innerHTML = `
            <div class="notification-dropdown-header">
                <h3>Notifications</h3>
                <div class="notification-dropdown-actions">
                    <button class="btn-mark-all-dropdown" title="Mark all as read">
                        <i class="fa-solid fa-check-double"></i>
                    </button>
                    <button class="btn-settings-dropdown" title="Settings">
                        <i class="fa-solid fa-gear"></i>
                    </button>
                </div>
            </div>
            <div class="notification-dropdown-content">
                <div class="notification-dropdown-loading">
                    <i class="fa-solid fa-spinner fa-spin"></i> Loading...
                </div>
            </div>
            <div class="notification-dropdown-footer">
                <a href="/notifications">View all notifications</a>
            </div>
        `;

        this.button.parentNode.appendChild(dropdown);
        this.dropdown = dropdown;

        // Attach dropdown event listeners
        this.dropdown.querySelector('.btn-mark-all-dropdown').addEventListener('click', () => {
            this.markAllAsRead();
        });

        this.dropdown.querySelector('.btn-settings-dropdown').addEventListener('click', () => {
            window.location.href = '/notification-preferences';
        });
    }

    attachEventListeners() {
        if (!this.button) return;

        // Toggle dropdown
        this.button.addEventListener('click', (e) => {
            e.stopPropagation();
            this.toggleDropdown();
        });

        // Close dropdown on outside click
        document.addEventListener('click', (e) => {
            if (this.isOpen && !this.dropdown.contains(e.target)) {
                this.closeDropdown();
            }
        });

        // Notification page handlers
        this.attachPageHandlers();
    }

    attachPageHandlers() {
        // Mark all read button on notifications page
        const markAllBtn = document.getElementById('mark-all-read');
        if (markAllBtn) {
            markAllBtn.addEventListener('click', () => this.markAllAsRead());
        }

        // Individual mark as read buttons
        document.addEventListener('click', (e) => {
            if (e.target.matches('.btn-mark-read') || e.target.closest('.btn-mark-read')) {
                const btn = e.target.closest('.btn-mark-read');
                const notificationId = parseInt(btn.dataset.id);
                this.markAsRead([notificationId]);
            }
        });

        // Load more button
        const loadMoreBtn = document.getElementById('load-more');
        if (loadMoreBtn) {
            loadMoreBtn.addEventListener('click', () => this.loadMoreNotifications());
        }
    }

    toggleDropdown() {
        if (this.isOpen) {
            this.closeDropdown();
        } else {
            this.openDropdown();
        }
    }

    openDropdown() {
        if (!this.dropdown) return;

        this.dropdown.classList.add('active');
        this.isOpen = true;
        this.loadNotifications();
    }

    closeDropdown() {
        if (!this.dropdown) return;

        this.dropdown.classList.remove('active');
        this.isOpen = false;
    }

    startRefreshInterval() {
        // Refresh notifications every 30 seconds
        this.refreshInterval = setInterval(() => {
            this.loadNotificationSummary();
        }, 30000);
    }

    stopRefreshInterval() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
            this.refreshInterval = null;
        }
    }

    async loadNotifications() {
        try {
            const response = await fetch('/api/notifications?limit=10');
            if (!response.ok) throw new Error('Failed to load notifications');

            const notifications = await response.json();
            this.notifications = notifications || [];
            this.updateDropdownContent();
            this.loadNotificationSummary();
        } catch (error) {
            console.error('Error loading notifications:', error);
            this.showDropdownError();
        }
    }

    async loadNotificationSummary() {
        try {
            const response = await fetch('/api/notification-summary');
            if (!response.ok) throw new Error('Failed to load notification summary');

            const summary = await response.json();
            this.updateBadge(summary.total_unread);
        } catch (error) {
            console.error('Error loading notification summary:', error);
        }
    }

    async loadMoreNotifications() {
        const loadMoreBtn = document.getElementById('load-more');
        if (!loadMoreBtn) return;

        try {
            loadMoreBtn.disabled = true;
            loadMoreBtn.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> Loading...';

            const currentCount = document.querySelectorAll('.notification-item').length;
            const response = await fetch(`/api/notifications?limit=20&offset=${currentCount}`);

            if (!response.ok) throw new Error('Failed to load more notifications');

            const notifications = await response.json();
            this.appendNotificationsToPage(notifications);

            if (notifications.length < 20) {
                loadMoreBtn.style.display = 'none';
            }
        } catch (error) {
            console.error('Error loading more notifications:', error);
        } finally {
            loadMoreBtn.disabled = false;
            loadMoreBtn.innerHTML = '<i class="fa-solid fa-chevron-down"></i> Load More';
        }
    }

    updateDropdownContent() {
        if (!this.dropdown) return;

        const content = this.dropdown.querySelector('.notification-dropdown-content');

        if (this.notifications.length === 0) {
            content.innerHTML = `
                <div class="notification-dropdown-empty">
                    <i class="fa-solid fa-bell-slash"></i>
                    <p>No notifications yet</p>
                </div>
            `;
            return;
        }

        content.innerHTML = this.notifications.map(notification => `
            <div class="notification-dropdown-item ${notification.read ? '' : 'unread'}" 
                 data-id="${notification.id}"
                 onclick="window.location.href='${notification.action_url}'">
                <div class="notification-dropdown-icon" style="color: ${notification.color}">
                    <i class="${notification.icon}"></i>
                </div>
                <div class="notification-dropdown-content">
                    <h4 class="notification-dropdown-title">${notification.title}</h4>
                    <p class="notification-dropdown-message">${notification.message}</p>
                    <span class="notification-dropdown-time">${notification.created_at_human}</span>
                </div>
            </div>
        `).join('');
    }

    showDropdownError() {
        if (!this.dropdown) return;

        const content = this.dropdown.querySelector('.notification-dropdown-content');
        content.innerHTML = `
            <div class="notification-dropdown-empty">
                <i class="fa-solid fa-exclamation-triangle"></i>
                <p>Failed to load notifications</p>
                <button onclick="notificationManager.loadNotifications()" 
                        style="margin-top: 8px; padding: 4px 8px; border: none; background: #3498db; color: white; border-radius: 4px; cursor: pointer;">
                    Retry
                </button>
            </div>
        `;
    }

    updateBadge(count) {
        if (!this.badge) return;

        this.unreadCount = count;
        this.badge.dataset.count = count;

        if (count > 0) {
            this.badge.style.display = 'block';
        } else {
            this.badge.style.display = 'none';
        }
    }

    async markAsRead(notificationIds) {
        try {
            const response = await fetch('/api/notifications/mark-read', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    notification_ids: notificationIds
                })
            });

            if (!response.ok) throw new Error('Failed to mark notifications as read');

            // Update UI
            notificationIds.forEach(id => {
                // Update dropdown
                const dropdownItem = this.dropdown?.querySelector(`[data-id="${id}"]`);
                if (dropdownItem) {
                    dropdownItem.classList.remove('unread');
                }

                // Update page
                const pageItem = document.querySelector(`.notification-item[data-id="${id}"]`);
                if (pageItem) {
                    pageItem.classList.remove('unread');
                    const markReadBtn = pageItem.querySelector('.btn-mark-read');
                    if (markReadBtn) {
                        markReadBtn.remove();
                    }
                }
            });

            // Update badge
            this.unreadCount = Math.max(0, this.unreadCount - notificationIds.length);
            this.updateBadge(this.unreadCount);

        } catch (error) {
            console.error('Error marking notifications as read:', error);
            alert('Failed to mark notifications as read. Please try again.');
        }
    }

    async markAllAsRead() {
        try {
            const response = await fetch('/api/notifications/mark-read', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    mark_all: true
                })
            });

            if (!response.ok) throw new Error('Failed to mark all notifications as read');

            // Update UI
            document.querySelectorAll('.notification-item.unread, .notification-dropdown-item.unread').forEach(item => {
                item.classList.remove('unread');
            });

            document.querySelectorAll('.btn-mark-read').forEach(btn => btn.remove());

            // Update badge
            this.updateBadge(0);

            // Reload notifications
            this.loadNotifications();

        } catch (error) {
            console.error('Error marking all notifications as read:', error);
            alert('Failed to mark all notifications as read. Please try again.');
        }
    }

    appendNotificationsToPage(notifications) {
        const notificationsList = document.getElementById('notifications-list');
        if (!notificationsList) return;

        notifications.forEach(notification => {
            const notificationElement = document.createElement('div');
            notificationElement.className = `notification-item ${notification.read ? '' : 'unread'}`;
            notificationElement.dataset.id = notification.id;

            notificationElement.innerHTML = `
                <div class="notification-icon" style="color: ${notification.color}">
                    <i class="${notification.icon}"></i>
                </div>
                <div class="notification-content">
                    <div class="notification-header">
                        <h4>${notification.title}</h4>
                        <span class="notification-time">${notification.created_at_human}</span>
                    </div>
                    <p class="notification-message">${notification.message}</p>
                    ${notification.actor_username ? `
                        <div class="notification-actor">
                            ${notification.actor_profile_url ?
                        `<img src="${notification.actor_profile_url}" alt="${notification.actor_username}" class="actor-avatar">` :
                        `<i class="fa-regular fa-user actor-avatar-icon"></i>`
                    }
                            <span>${notification.actor_username}</span>
                        </div>
                    ` : ''}
                </div>
                <div class="notification-actions">
                    ${notification.action_url ? `
                        <a href="${notification.action_url}" class="btn-view">
                            <i class="fa-solid fa-eye"></i> View
                        </a>
                    ` : ''}
                    ${!notification.read ? `
                        <button class="btn-mark-read" data-id="${notification.id}">
                            <i class="fa-solid fa-check"></i>
                        </button>
                    ` : ''}
                </div>
            `;

            notificationsList.appendChild(notificationElement);
        });
    }

    destroy() {
        this.stopRefreshInterval();
        if (this.dropdown) {
            this.dropdown.remove();
        }
        if (this.button && this.button.parentNode) {
            this.button.parentNode.remove();
        }
    }
}

// Initialize notification manager when DOM is loaded
let notificationManager;

document.addEventListener('DOMContentLoaded', function () {
    // Only initialize if user is logged in
    const isLoggedIn = document.querySelector('.authentication .username');
    if (isLoggedIn) {
        notificationManager = new NotificationManager();
    }
});

// Clean up on page unload
window.addEventListener('beforeunload', function () {
    if (notificationManager) {
        notificationManager.destroy();
    }
});