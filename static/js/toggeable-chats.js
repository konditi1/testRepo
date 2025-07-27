// Toggleable Chat JavaScript
class ToggleableChat {
    constructor() {
        this.currentChatId = null;
        this.ws = null;
        this.isTyping = false;
        this.typingTimeout = null;
        this.userID = null;
        this.username = null;
        this.isConnected = false;
        
        this.init();
    }

    init() {
        this.createChatPopupHTML();
        this.bindEvents();
        this.getUserInfo();
    }

    getUserInfo() {
        // Get user info from existing elements
        const userElement = document.querySelector('.username');
        if (userElement) {
            this.username = userElement.textContent.trim();
        }
        
        // Try to extract user ID from existing data or make an API call
        const userIdElement = document.querySelector('[data-user-id]');
        if (userIdElement) {
            this.userID = parseInt(userIdElement.getAttribute('data-user-id'));
        }
    }

    createChatPopupHTML() {
        const chatOverlay = document.createElement('div');
        chatOverlay.className = 'chat-overlay';
        chatOverlay.id = 'chatOverlay';

        const chatPopup = document.createElement('div');
        chatPopup.className = 'chat-popup';
        chatPopup.id = 'chatPopup';

        chatPopup.innerHTML = `
            <div class="chat-popup-header">
                <div class="chat-popup-header-left">
                    <div class="chat-popup-recipient-avatar-container">
                        <img id="chatPopupRecipientAvatar" class="chat-popup-recipient-avatar" style="display: none;">
                        <div id="chatPopupInitialsAvatar" class="chat-popup-initials-avatar" style="display: none;"></div>
                    </div>
                    <div class="chat-popup-header-info">
                        <h3 id="chatPopupRecipientName">Select a user</h3>
                        <p class="chat-popup-user-status" id="chatPopupUserStatus">offline</p>
                    </div>
                </div>
                <div class="chat-popup-actions">
                    <button class="chat-popup-btn" id="chatPopupVideoBtn" title="Video call">
                        <i class="fas fa-video"></i>
                    </button>
                    <button class="chat-popup-btn" id="chatPopupVoiceBtn" title="Voice call">
                        <i class="fas fa-phone"></i>
                    </button>
                    <button class="chat-popup-btn chat-popup-close" id="chatPopupClose" title="Close chat">
                        <i class="fas fa-times"></i>
                    </button>
                </div>
            </div>
            
            <div class="chat-popup-status" id="chatPopupStatus">
                <i class="fas fa-circle-notch fa-spin status-icon" id="chatPopupStatusIcon"></i>
                <span id="chatPopupConnectionStatus">Not connected</span>
            </div>
            
            <div class="chat-popup-messages" id="chatPopupMessages">
                <div style="text-align: center; padding: 40px; color: var(--secondary-color);">
                    <i class="fas fa-comments" style="font-size: 48px; margin-bottom: 16px; display: block;"></i>
                    <p>Select a user from the sidebar to start chatting</p>
                </div>
            </div>
            
            <div class="chat-popup-input">
                <div class="chat-popup-input-wrapper">
                    <input type="text" id="chatPopupMessageInput" placeholder="Type a message..." disabled>
                </div>
                <button class="chat-popup-send-btn" id="chatPopupSendBtn" title="Send message" disabled>
                    <i class="fas fa-paper-plane"></i>
                </button>
            </div>
        `;

        document.body.appendChild(chatOverlay);
        document.body.appendChild(chatPopup);
    }

    bindEvents() {
        // Chat user links in sidebar
        document.addEventListener('click', (e) => {
            const chatUserLink = e.target.closest('.chat-user-link');
            if (chatUserLink) {
                e.preventDefault();
                const recipientId = this.extractRecipientId(chatUserLink.href);
                const recipientUsername = chatUserLink.querySelector('span:last-child')?.textContent.trim();
                const profileImg = chatUserLink.querySelector('img');
                const profileUrl = profileImg ? profileImg.src : null;
                
                this.openChat(recipientId, recipientUsername, profileUrl);
            }
        });

        // Close chat popup
        document.addEventListener('click', (e) => {
            if (e.target.matches('#chatPopupClose') || e.target.closest('#chatPopupClose')) {
                this.closeChat();
            }
            
            // Close when clicking overlay
            if (e.target.matches('#chatOverlay')) {
                this.closeChat();
            }
        });

        // Send message
        document.addEventListener('click', (e) => {
            if (e.target.matches('#chatPopupSendBtn') || e.target.closest('#chatPopupSendBtn')) {
                this.sendMessage();
            }
        });

        // Enter key to send message
        document.addEventListener('keypress', (e) => {
            if (e.target.matches('#chatPopupMessageInput') && e.key === 'Enter') {
                this.sendMessage();
            }
        });

        // Typing indicators
        document.addEventListener('input', (e) => {
            if (e.target.matches('#chatPopupMessageInput')) {
                this.handleTyping();
            }
        });

        document.addEventListener('blur', (e) => {
            if (e.target.matches('#chatPopupMessageInput')) {
                this.stopTyping();
            }
        });

        // Video and voice call buttons
        document.addEventListener('click', (e) => {
            if (e.target.matches('#chatPopupVideoBtn') || e.target.closest('#chatPopupVideoBtn')) {
                this.startVideoCall();
            }
            if (e.target.matches('#chatPopupVoiceBtn') || e.target.closest('#chatPopupVoiceBtn')) {
                this.startVoiceCall();
            }
        });

        // ESC key to close
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && this.isChatOpen()) {
                this.closeChat();
            }
        });
    }

    extractRecipientId(href) {
        const url = new URL(href, window.location.origin);
        return url.searchParams.get('recipient_id');
    }

    async openChat(recipientId, recipientUsername, profileUrl = null) {
        if (!recipientId || !recipientUsername) {
            console.error('Missing recipient information');
            return;
        }

        this.currentChatId = recipientId;
        
        // Update chat header
        this.updateChatHeader(recipientUsername, profileUrl);
        
        // Show chat popup
        this.showChatPopup();
        
        // Enable input
        this.enableChatInput();
        
        // Load chat messages
        await this.loadChatMessages(recipientId);
        
        // Initialize WebSocket if not already connected
        if (!this.ws || this.ws.readyState === WebSocket.CLOSED) {
            this.initializeWebSocket();
        }
    }

    updateChatHeader(recipientUsername, profileUrl = null) {
        const nameElement = document.getElementById('chatPopupRecipientName');
        const avatarImg = document.getElementById('chatPopupRecipientAvatar');
        const initialsAvatar = document.getElementById('chatPopupInitialsAvatar');
        const statusElement = document.getElementById('chatPopupUserStatus');

        nameElement.textContent = recipientUsername;
        
        if (profileUrl) {
            avatarImg.src = profileUrl;
            avatarImg.style.display = 'block';
            initialsAvatar.style.display = 'none';
        } else {
            // Create initials avatar
            const initials = this.getInitials(recipientUsername);
            const color = this.generateAvatarColor(recipientUsername);
            
            initialsAvatar.textContent = initials;
            initialsAvatar.style.backgroundColor = color;
            initialsAvatar.style.display = 'flex';
            avatarImg.style.display = 'none';
        }
        
        statusElement.textContent = 'offline';
    }

    getInitials(username) {
        if (!username) return '?';
        
        const parts = username.split(' ');
        if (parts.length >= 2) {
            return parts[0].charAt(0) + parts[parts.length - 1].charAt(0);
        } else if (username.length > 0) {
            return username.length >= 2 ? username.substring(0, 2) : username;
        }
        return '?';
    }

    generateAvatarColor(username) {
        let hash = 0;
        for (let i = 0; i < username.length; i++) {
            hash = username.charCodeAt(i) + ((hash << 5) - hash);
        }
        const hue = Math.abs(hash % 360);
        return `hsl(${hue}, 70%, 60%)`;
    }

    showChatPopup() {
        const overlay = document.getElementById('chatOverlay');
        const popup = document.getElementById('chatPopup');
        
        overlay.classList.add('active');
        popup.classList.add('active');
        
        // Focus on message input after animation
        setTimeout(() => {
            document.getElementById('chatPopupMessageInput')?.focus();
        }, 300);
    }

    closeChat() {
        const overlay = document.getElementById('chatOverlay');
        const popup = document.getElementById('chatPopup');
        
        overlay.classList.remove('active');
        popup.classList.remove('active');
        
        // Clean up
        this.currentChatId = null;
        this.disableChatInput();
        this.clearMessages();
        
        // Close WebSocket if open
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.close();
        }
    }

    isChatOpen() {
        const popup = document.getElementById('chatPopup');
        return popup && popup.classList.contains('active');
    }

    enableChatInput() {
        const input = document.getElementById('chatPopupMessageInput');
        const sendBtn = document.getElementById('chatPopupSendBtn');
        
        if (input) input.disabled = false;
        if (sendBtn) sendBtn.disabled = false;
    }

    disableChatInput() {
        const input = document.getElementById('chatPopupMessageInput');
        const sendBtn = document.getElementById('chatPopupSendBtn');
        
        if (input) {
            input.disabled = true;
            input.value = '';
        }
        if (sendBtn) sendBtn.disabled = true;
    }

    clearMessages() {
        const messagesContainer = document.getElementById('chatPopupMessages');
        if (messagesContainer) {
            messagesContainer.innerHTML = `
                <div style="text-align: center; padding: 40px; color: var(--secondary-color);">
                    <i class="fas fa-comments" style="font-size: 48px; margin-bottom: 16px; display: block;"></i>
                    <p>Select a user from the sidebar to start chatting</p>
                </div>
            `;
        }
    }

    async loadChatMessages(recipientId) {
        const messagesContainer = document.getElementById('chatPopupMessages');
        
        // Show loading
        messagesContainer.innerHTML = `
            <div style="text-align: center; padding: 40px; color: var(--secondary-color);">
                <i class="fas fa-spinner fa-spin" style="font-size: 24px; margin-bottom: 16px; display: block;"></i>
                <p>Loading messages...</p>
            </div>
        `;

        try {
            const response = await fetch(`/api/chat-messages?recipient_id=${recipientId}`);
            if (!response.ok) {
                throw new Error('Failed to load messages');
            }
            
            const data = await response.json();
            this.displayMessages(data.messages || []);
        } catch (error) {
            console.error('Error loading messages:', error);
            messagesContainer.innerHTML = `
                <div style="text-align: center; padding: 40px; color: #ff6b6b;">
                    <i class="fas fa-exclamation-triangle" style="font-size: 24px; margin-bottom: 16px; display: block;"></i>
                    <p>Failed to load messages</p>
                </div>
            `;
        }
    }

    displayMessages(messages) {
        const messagesContainer = document.getElementById('chatPopupMessages');
        
        if (!messages || messages.length === 0) {
            messagesContainer.innerHTML = `
                <div style="text-align: center; padding: 40px; color: var(--secondary-color);">
                    <i class="fas fa-comment" style="font-size: 48px; margin-bottom: 16px; display: block;"></i>
                    <p>No messages yet. Start the conversation!</p>
                </div>
            `;
            return;
        }

        // Group messages by date
        const messagesByDate = this.groupMessagesByDate(messages);
        
        let html = '';
        for (const [date, msgs] of Object.entries(messagesByDate)) {
            html += `<div class="date-header">${date}</div>`;
            html += '<div class="message-group">';
            
            for (const msg of msgs) {
                const isSent = msg.sender_id == this.userID;
                const time = new Date(msg.created_at).toLocaleTimeString('en-US', {
                    hour: '2-digit',
                    minute: '2-digit',
                    hour12: false
                });
                
                html += `
                    <div class="message ${isSent ? 'sent' : 'received'}" data-message-id="${msg.id}">
                        <p>${this.escapeHtml(msg.content)}</p>
                        <span class="timestamp">${time}</span>
                    </div>
                `;
            }
            
            html += '</div>';
        }
        
        messagesContainer.innerHTML = html;
        this.scrollToBottom();
    }

    groupMessagesByDate(messages) {
        const groups = {};
        const now = new Date();
        
        messages.forEach(msg => {
            const msgDate = new Date(msg.created_at);
            let dateKey;
            
            if (msgDate.toDateString() === now.toDateString()) {
                dateKey = 'Today';
            } else {
                const yesterday = new Date(now);
                yesterday.setDate(now.getDate() - 1);
                if (msgDate.toDateString() === yesterday.toDateString()) {
                    dateKey = 'Yesterday';
                } else {
                    dateKey = msgDate.toLocaleDateString('en-US', { 
                        weekday: 'long', 
                        year: 'numeric', 
                        month: 'long', 
                        day: 'numeric' 
                    });
                }
            }
            
            if (!groups[dateKey]) {
                groups[dateKey] = [];
            }
            groups[dateKey].push(msg);
        });
        
        return groups;
    }

    initializeWebSocket() {
        if (!this.userID) {
            console.error('User ID not available for WebSocket connection');
            return;
        }

        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        this.ws = new WebSocket(protocol + '//' + window.location.host + '/ws');
        
        const statusElement = document.getElementById('chatPopupConnectionStatus');
        const chatStatus = document.getElementById('chatPopupStatus');
        const statusIcon = document.getElementById('chatPopupStatusIcon');
        
        this.ws.onopen = () => {
            console.log('WebSocket connection established');
            this.isConnected = true;
            statusElement.textContent = 'Connected';
            chatStatus.className = 'chat-popup-status connected';
            statusIcon.className = 'fas fa-check-circle status-icon';
            
            // Hide status after 3 seconds
            setTimeout(() => {
                chatStatus.style.display = 'none';
            }, 3000);
        };
        
        this.ws.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                
                if (message.type === 'typing') {
                    this.handleTypingIndicator(message);
                } else if (message.type === 'status_update') {
                    this.handleStatusUpdate(message);
                } else if (message.sender_id && message.recipient_id && message.content) {
                    this.handleIncomingMessage(message);
                }
            } catch (e) {
                console.error('Error parsing WebSocket message:', e);
            }
        };
        
        this.ws.onclose = () => {
            console.log('WebSocket connection closed');
            this.isConnected = false;
            statusElement.textContent = 'Disconnected. Reconnecting...';
            chatStatus.className = 'chat-popup-status disconnected';
            chatStatus.style.display = 'flex';
            statusIcon.className = 'fas fa-circle-notch fa-spin status-icon';
            
            // Attempt to reconnect after 3 seconds
            if (this.isChatOpen()) {
                setTimeout(() => this.initializeWebSocket(), 3000);
            }
        };
        
        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            statusElement.textContent = 'Connection error';
            chatStatus.className = 'chat-popup-status disconnected';
            chatStatus.style.display = 'flex';
            statusIcon.className = 'fas fa-exclamation-triangle status-icon';
        };
    }

    sendMessage() {
        const input = document.getElementById('chatPopupMessageInput');
        const content = input.value.trim();
        
        if (!content || !this.ws || this.ws.readyState !== WebSocket.OPEN || !this.currentChatId) {
            return;
        }

        const sendBtn = document.getElementById('chatPopupSendBtn');
        const sendIcon = sendBtn.querySelector('i');
        
        // Show sending state
        sendBtn.disabled = true;
        sendIcon.className = 'fas fa-circle-notch fa-spin';
        
        const message = {
            sender_id: this.userID,
            recipient_id: parseInt(this.currentChatId),
            content: content,
            created_at: new Date().toISOString(),
            sender_username: this.username
        };

        try {
            this.ws.send(JSON.stringify(message));
            input.value = '';
            
            // Reset send button after a delay
            setTimeout(() => {
                sendBtn.disabled = false;
                sendIcon.className = 'fas fa-paper-plane';
            }, 500);
            
        } catch (error) {
            console.error('Error sending message:', error);
            sendBtn.disabled = false;
            sendIcon.className = 'fas fa-paper-plane';
        }
    }

    handleIncomingMessage(message) {
        // Only handle messages for the current chat
        if (message.sender_id != this.currentChatId && message.recipient_id != this.currentChatId) {
            return;
        }

        this.appendMessage(message);
    }

    appendMessage(message) {
        const messagesContainer = document.getElementById('chatPopupMessages');
        
        // Check if this is an empty state and replace it
        const emptyState = messagesContainer.querySelector('div[style*="text-align: center"]');
        if (emptyState && emptyState.textContent.includes('No messages yet')) {
            messagesContainer.innerHTML = '';
        }

        const isSent = message.sender_id == this.userID;
        const time = new Date(message.created_at).toLocaleTimeString('en-US', {
            hour: '2-digit',
            minute: '2-digit',
            hour12: false
        });

        // Check if we need a new date header
        const lastDateHeader = messagesContainer.querySelector('.date-header:last-of-type');
        const today = new Date().toDateString() === new Date(message.created_at).toDateString() ? 'Today' : 
                     new Date(message.created_at).toLocaleDateString();
        
        if (!lastDateHeader || lastDateHeader.textContent !== today) {
            const dateHeader = document.createElement('div');
            dateHeader.className = 'date-header';
            dateHeader.textContent = today;
            messagesContainer.appendChild(dateHeader);
            
            const messageGroup = document.createElement('div');
            messageGroup.className = 'message-group';
            messagesContainer.appendChild(messageGroup);
        }

        // Find the last message group
        let messageGroup = messagesContainer.querySelector('.message-group:last-child');
        if (!messageGroup) {
            messageGroup = document.createElement('div');
            messageGroup.className = 'message-group';
            messagesContainer.appendChild(messageGroup);
        }

        const messageDiv = document.createElement('div');
        messageDiv.className = `message ${isSent ? 'sent' : 'received'}`;
        messageDiv.setAttribute('data-message-id', message.id || Date.now());
        
        messageDiv.innerHTML = `
            <p>${this.escapeHtml(message.content)}</p>
            <span class="timestamp">${time}</span>
        `;

        messageGroup.appendChild(messageDiv);
        this.scrollToBottom();
    }

    handleTyping() {
        if (!this.isTyping && this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.isTyping = true;
            this.sendTypingIndicator(true);
        }
        
        clearTimeout(this.typingTimeout);
        this.typingTimeout = setTimeout(() => {
            this.stopTyping();
        }, 1000);
    }

    stopTyping() {
        if (this.isTyping && this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.isTyping = false;
            this.sendTypingIndicator(false);
        }
        clearTimeout(this.typingTimeout);
    }

    sendTypingIndicator(isTyping) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN && this.currentChatId) {
            this.ws.send(JSON.stringify({
                type: 'typing',
                sender_id: this.userID,
                recipient_id: parseInt(this.currentChatId),
                is_typing: isTyping
            }));
        }
    }

    handleTypingIndicator(message) {
        // Only show typing indicator for current chat
        if (message.sender_id != this.currentChatId) {
            return;
        }

        // For now, we'll skip the typing indicator in the popup
        // You can implement it similar to the main chat if needed
    }

    handleStatusUpdate(message) {
        if (message.user_id == this.currentChatId) {
            const statusElement = document.getElementById('chatPopupUserStatus');
            if (statusElement) {
                statusElement.textContent = message.is_online ? 'online' : 'offline';
            }
        }
    }

    scrollToBottom() {
        const messagesContainer = document.getElementById('chatPopupMessages');
        if (messagesContainer) {
            messagesContainer.scrollTop = messagesContainer.scrollHeight;
        }
    }

    startVideoCall() {
        if (this.currentChatId) {
            alert('Video call feature coming soon!');
        }
    }

    startVoiceCall() {
        if (this.currentChatId) {
            alert('Voice call feature coming soon!');
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

// Initialize toggleable chat when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    window.toggleableChat = new ToggleableChat();
});