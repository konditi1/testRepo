document.addEventListener('DOMContentLoaded', () => {
    const chatPanel = document.getElementById('chat-panel');
    const chatToggleBtn = document.querySelector('.chat-toggle-btn');
    const chatCloseBtn = document.querySelector('.chat-close-btn');
    const chatContent = document.getElementById('chat-content');
    const chatUsers = document.querySelectorAll('.chat-user');
    let ws = null;
    let currentRecipientID = null;

    // Toggle chat panel
    chatToggleBtn.addEventListener('click', () => {
        chatPanel.classList.toggle('open');
    });

    chatCloseBtn.addEventListener('click', () => {
        chatPanel.classList.remove('open');
        if (ws) {
            ws.close();
            ws = null;
        }
        chatContent.style.display = 'none';
        currentRecipientID = null;
    });

    // Handle user selection
    chatUsers.forEach(user => {
        user.addEventListener('click', () => {
            const userID = parseInt(user.dataset.userId);
            const username = user.dataset.username;
            const avatar = user.querySelector('.chat-user-avatar')?.src || '/static/images/default-avatar.png';

            // Update chat header
            document.getElementById('recipient-username').textContent = username;
            document.getElementById('recipient-avatar').src = avatar;
            chatContent.style.display = 'flex';

            if (currentRecipientID !== userID) {
                currentRecipientID = userID;
                loadMessages(userID);
                initializeWebSocket(userID);
            }
        });
    });

    function initializeWebSocket(recipientID) {
        if (ws) {
            ws.close();
        }

        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        ws = new WebSocket(protocol + '//' + window.location.host + '/ws');

        const statusElement = document.getElementById('connection-status');
        const spinnerElement = document.getElementById('connection-spinner');

        ws.onopen = () => {
            console.log('WebSocket connected');
            statusElement.textContent = 'Connected';
            statusElement.classList.add('connected');
            spinnerElement.classList.remove('active');
        };

        ws.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                if (message.sender_id && message.recipient_id && message.content) {
                    appendMessage(message);
                }
            } catch (e) {
                console.error('Error parsing message:', e);
            }
        };

        ws.onclose = () => {
            console.log('WebSocket closed');
            statusElement.textContent = 'Disconnected. Reconnecting...';
            statusElement.classList.remove('connected');
            spinnerElement.classList.add('active');
            setTimeout(() => initializeWebSocket(recipientID), 3000);
        };

        ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            statusElement.textContent = 'Error connecting';
            statusElement.classList.remove('connected');
            spinnerElement.classList.add('active');
        };
    }

    function loadMessages(recipientID) {
        fetch(`/api/messages?recipient_id=${recipientID}`)
            .then(response => response.json())
            .then(data => {
                const messagesDiv = document.getElementById('chat-messages');
                messagesDiv.innerHTML = '';
                data.messages.forEach(group => {
                    const dateHeader = document.createElement('div');
                    dateHeader.className = 'date-header';
                    dateHeader.textContent = group.date;
                    messagesDiv.appendChild(dateHeader);

                    const messageGroup = document.createElement('div');
                    messageGroup.className = 'message-group';
                    group.messages.forEach(msg => {
                        appendMessage(msg, messageGroup);
                    });
                    messagesDiv.appendChild(messageGroup);
                });
                messagesDiv.scrollTop = messagesDiv.scrollHeight;
            })
            .catch(error => console.error('Error loading messages:', error));
    }

    function appendMessage(message, messageGroup = null) {
        const messagesDiv = document.getElementById('chat-messages');
        const existingMessage = document.querySelector(`[data-message-id="${message.id}"]`);
        if (existingMessage) return;

        const today = new Date();
        const messageDate = new Date(message.created_at);
        const dateStr = messageDate.toDateString() === today.toDateString() ? 'Today' :
                        messageDate.toDateString() === new Date(today.setDate(today.getDate() - 1)).toDateString() ? 'Yesterday' :
                        messageDate.toLocaleDateString('en-US', { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' });

        let dateHeader = Array.from(messagesDiv.querySelectorAll('.date-header')).find(header => header.textContent === dateStr);
        let targetGroup = messageGroup || messagesDiv.querySelector(`.date-header:last-of-type + .message-group`);

        if (!dateHeader && !messageGroup) {
            dateHeader = document.createElement('div');
            dateHeader.className = 'date-header';
            dateHeader.textContent = dateStr;
            messagesDiv.appendChild(dateHeader);
            targetGroup = document.createElement('div');
            targetGroup.className = 'message-group';
            messagesDiv.appendChild(targetGroup);
        } else if (!targetGroup && !messageGroup) {
            targetGroup = document.createElement('div');
            targetGroup.className = 'message-group';
            dateHeader.insertAdjacentElement('afterend', targetGroup);
        }

        const messageDiv = document.createElement('div');
        messageDiv.className = `message ${message.sender_id === userID ? 'sent' : 'received'}`;
        messageDiv.setAttribute('data-message-id', message.id);
        messageDiv.setAttribute('data-timestamp', message.created_at);

        const formattedTime = messageDate.toLocaleTimeString('en-US', {
            hour: '2-digit',
            minute: '2-digit',
            hour12: false
        });

        messageDiv.innerHTML = `
            <div class="message-content">
                <p>${message.content.replace(/</g, '&lt;').replace(/>/g, '&gt;')}</p>
                <span class="timestamp">${formattedTime}</span>
            </div>
        `;

        const messages = targetGroup.querySelectorAll('.message');
        let inserted = false;
        for (let i = 0; i < messages.length; i++) {
            const existingTimestamp = new Date(messages[i].getAttribute('data-timestamp'));
            if (new Date(message.created_at) < existingTimestamp) {
                targetGroup.insertBefore(messageDiv, messages[i]);
                inserted = true;
                break;
            }
        }
        if (!inserted) {
            targetGroup.appendChild(messageDiv);
        }

        if (!messageGroup) {
            messagesDiv.scrollTop = messagesDiv.scrollHeight;
        }
    }

    window.sendMessage = function() {
        const input = document.getElementById('message-input');
        const content = input.value.trim();
        if (content === '' || !ws || ws.readyState !== WebSocket.OPEN || !currentRecipientID) return;

        const msg = {
            sender_id: userID,
            recipient_id: currentRecipientID,
            content: content,
            created_at: new Date().toISOString(),
            sender_username: username,
            recipient_username: document.getElementById('recipient-username').textContent
        };

        try {
            ws.send(JSON.stringify(msg));
            input.value = '';
            appendMessage({ ...msg, id: Date.now() }); // Temporary ID
        } catch (e) {
            console.error('Error sending message:', e);
        }
    };

    const messageInput = document.getElementById('message-input');
    messageInput.addEventListener('keypress', (e) => {
        if (e.key === 'Enter') {
            sendMessage();
        }
    });
});