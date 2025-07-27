// static/js/social-integration.js
document.addEventListener('DOMContentLoaded', function () {
    console.log('Social integration script loaded');
    initSocialSharing();
    initShareTracking();
});

function initSocialSharing() {
    console.log('Initializing social sharing');
    
    // Check if user is authenticated
    const isAuthenticated = document.querySelector('.authentication .logout-button') !== null;
    if (!isAuthenticated) {
        console.log('User not authenticated - social sharing disabled');
        return;
    }
    
    // Add share buttons to all posts, questions, and documents
    addShareButtons();
    
    // Handle share button clicks
    document.addEventListener('click', function(e) {
        if (e.target.classList.contains('btn-share') || e.target.closest('.btn-share')) {
            e.preventDefault();
            const shareBtn = e.target.closest('.btn-share') || e.target;
            console.log('Share button clicked:', shareBtn);
            handleShareClick(shareBtn);
        }
    });
}

function addShareButtons() {
    const shareButtons = document.querySelectorAll('.btn-share');
    console.log('Found share buttons:', shareButtons.length);

    shareButtons.forEach(shareBtn => {
        if (!shareBtn.classList.contains('enhanced')) {
            enhanceShareButton(shareBtn);
        }
    });
}

function enhanceShareButton(shareBtn) {
    console.log('Enhancing share button:', shareBtn);
    shareBtn.classList.add('enhanced');

    // Get content info from data attributes or fallback methods
    const contentType = shareBtn.dataset.contentType || getContentTypeFromContext(shareBtn);
    const contentId = shareBtn.dataset.contentId || getContentIdFromContext(shareBtn);

    console.log('Content info:', { contentType, contentId });

    shareBtn.dataset.contentType = contentType;
    shareBtn.dataset.contentId = contentId;

    // Create share dropdown
    const shareDropdown = createShareDropdown();
    shareBtn.appendChild(shareDropdown);

    // Ensure the button has the right styling
    if (!shareBtn.querySelector('span')) {
        const icon = shareBtn.querySelector('i');
        if (icon) {
            shareBtn.innerHTML = '';
            shareBtn.appendChild(icon);
            const span = document.createElement('span');
            span.textContent = 'Share';
            shareBtn.appendChild(span);
            shareBtn.appendChild(shareDropdown);
        }
    }
}

function createShareDropdown() {
    const dropdown = document.createElement('div');
    dropdown.className = 'share-dropdown';
    dropdown.style.display = 'none';

    dropdown.innerHTML = `
        <div class="share-options">
            <div class="share-option" data-platform="twitter">
                <i class="fa-brands fa-twitter"></i>
                <span>Twitter</span>
            </div>
            <div class="share-option" data-platform="linkedin">
                <i class="fa-brands fa-linkedin"></i>
                <span>LinkedIn</span>
            </div>
            <div class="share-option" data-platform="facebook">
                <i class="fa-brands fa-facebook"></i>
                <span>Facebook</span>
            </div>
            <div class="share-option" data-platform="whatsapp">
                <i class="fa-brands fa-whatsapp"></i>
                <span>WhatsApp</span>
            </div>
            <div class="share-option" data-platform="email">
                <i class="fa-solid fa-envelope"></i>
                <span>Email</span>
            </div>
            <div class="share-option copy-link" data-platform="copy">
                <i class="fa-solid fa-link"></i>
                <span>Copy Link</span>
            </div>
        </div>
    `;

    return dropdown;
}

function handleShareClick(shareBtn) {
    console.log('Handling share click for button:', shareBtn);

    const dropdown = shareBtn.querySelector('.share-dropdown');
    if (!dropdown) {
        console.error('No dropdown found for share button');
        return;
    }

    const isVisible = dropdown.style.display !== 'none';

    // Hide all other dropdowns
    document.querySelectorAll('.share-dropdown').forEach(d => {
        d.style.display = 'none';
        d.classList.remove('show');
    });

    // Toggle current dropdown
    if (isVisible) {
        dropdown.style.display = 'none';
        dropdown.classList.remove('show');
    } else {
        dropdown.style.display = 'block';
        // Use setTimeout to allow display change to take effect
        setTimeout(() => {
            dropdown.classList.add('show');
        }, 10);
    }

    // Handle platform selection (remove any existing listeners first)
    const existingHandler = dropdown._clickHandler;
    if (existingHandler) {
        dropdown.removeEventListener('click', existingHandler);
    }

    const clickHandler = function (e) {
        e.stopPropagation();
        const option = e.target.closest('.share-option');
        if (option) {
            const platform = option.dataset.platform;
            const contentType = shareBtn.dataset.contentType;
            const contentId = shareBtn.dataset.contentId;

            console.log('Platform selected:', { platform, contentType, contentId });

            handlePlatformShare(platform, contentType, contentId, shareBtn);
            dropdown.style.display = 'none';
            dropdown.classList.remove('show');
        }
    };

    dropdown._clickHandler = clickHandler;
    dropdown.addEventListener('click', clickHandler);
}

function handlePlatformShare(platform, contentType, contentId, shareBtn) {
    console.log('Handling platform share:', { platform, contentType, contentId });

    // Check if user is authenticated before proceeding
    const isAuthenticated = document.querySelector('.authentication .logout-button') !== null;
    if (!isAuthenticated) {
        showShareError('Please log in to share content');
        return;
    }

    if (platform === 'copy') {
        copyContentLink(contentType, contentId);
        return;
    }

    // Show loading state
    const originalContent = shareBtn.innerHTML;
    shareBtn.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i><span>Sharing...</span>';

    // Use the single authenticated endpoint
    const endpoint = '/api/share-content';

    // Send share request to backend
    fetch(endpoint, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            content_type: contentType,
            content_id: parseInt(contentId),
            platform: platform
        })
    })
        .then(response => {
            console.log('Share response status:', response.status);
            return response.json();
        })
        .then(data => {
            console.log('Share response data:', data);
            if (data.success) {
                // Open share URL in new window
                window.open(data.share_url, '_blank', 'width=600,height=400,scrollbars=yes,resizable=yes');

                // Show success feedback
                showShareSuccess(platform);

                // Update share count if available
                updateShareCount(shareBtn);
            } else {
                if (data.error && data.error.includes('Authentication required')) {
                    showShareError('Please log in to share content');
                } else {
                    showShareError(data.error || 'Failed to generate share link');
                }
            }
        })
        .catch(error => {
            console.error('Share error:', error);
            showShareError('Network error occurred');
        })
        .finally(() => {
            // Restore button state
            shareBtn.innerHTML = originalContent;
            // Re-enhance the button since we replaced its content
            setTimeout(() => {
                if (!shareBtn.classList.contains('enhanced')) {
                    enhanceShareButton(shareBtn);
                }
            }, 100);
        });
}

function copyContentLink(contentType, contentId) {
    const baseURL = window.location.origin;
    let linkURL;

    switch (contentType) {
        case 'post':
            linkURL = `${baseURL}/view-post?id=${contentId}`;
            break;
        case 'question':
            linkURL = `${baseURL}/view-question?id=${contentId}`;
            break;
        case 'document':
            linkURL = `${baseURL}/view-document?id=${contentId}`;
            break;
        default:
            linkURL = baseURL;
    }

    console.log('Copying link:', linkURL);

    if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(linkURL).then(() => {
            showNotification('Link copied to clipboard!', 'success');
        }).catch(err => {
            console.error('Failed to copy link:', err);
            fallbackCopyTextToClipboard(linkURL);
        });
    } else {
        fallbackCopyTextToClipboard(linkURL);
    }
}

function fallbackCopyTextToClipboard(text) {
    const textArea = document.createElement('textarea');
    textArea.value = text;
    textArea.style.position = 'fixed';
    textArea.style.left = '-999999px';
    textArea.style.top = '-999999px';
    document.body.appendChild(textArea);
    textArea.focus();
    textArea.select();

    try {
        document.execCommand('copy');
        showNotification('Link copied to clipboard!', 'success');
    } catch (err) {
        console.error('Fallback: Could not copy text:', err);
        showNotification('Could not copy link', 'error');
    }

    document.body.removeChild(textArea);
}

function getContentTypeFromContext(shareBtn) {
    // Try to find content type from various sources
    const postCard = shareBtn.closest('.post-card');
    if (postCard) {
        const id = postCard.id;
        if (id.startsWith('post-')) {
            return 'post';
        } else if (id.startsWith('question-')) {
            return 'question';
        } else if (id.startsWith('document-')) {
            return 'document';
        }
    }

    // Fallback: try to extract from nearby buttons
    const container = shareBtn.closest('.reaction');
    if (container) {
        const likeBtn = container.querySelector('[data-id]');
        if (likeBtn) {
            if (likeBtn.classList.contains('btn-like-question')) {
                return 'question';
            } else if (likeBtn.classList.contains('btn-like-document')) {
                return 'document';
            } else {
                return 'post';
            }
        }
    }

    return 'post'; // default fallback
}

function getContentIdFromContext(shareBtn) {
    // Try to find content ID from various sources
    const postCard = shareBtn.closest('.post-card');
    if (postCard) {
        const id = postCard.id;
        const parts = id.split('-');
        if (parts.length > 1) {
            return parts[1];
        }
    }

    // Fallback: try to extract from nearby buttons
    const container = shareBtn.closest('.reaction');
    if (container) {
        const likeBtn = container.querySelector('[data-id]');
        if (likeBtn) {
            return likeBtn.dataset.id;
        }
    }

    return '0'; // fallback
}

function initShareTracking() {
    // Track share button visibility
    if ('IntersectionObserver' in window) {
        const observer = new IntersectionObserver((entries) => {
            entries.forEach(entry => {
                if (entry.isIntersecting) {
                    const shareBtn = entry.target;
                    if (!shareBtn.dataset.tracked) {
                        shareBtn.dataset.tracked = 'true';
                        // Could send analytics event here
                        console.log('Share button became visible:', shareBtn);
                    }
                }
            });
        });

        document.querySelectorAll('.btn-share').forEach(btn => {
            observer.observe(btn);
        });
    }
}

function updateShareCount(shareBtn) {
    // This could be used to update share counts if you decide to track them
    const countSpan = shareBtn.querySelector('.share-count');
    if (countSpan) {
        const currentCount = parseInt(countSpan.textContent) || 0;
        countSpan.textContent = currentCount + 1;
    }
}

function showShareSuccess(platform) {
    const platformNames = {
        twitter: 'Twitter',
        linkedin: 'LinkedIn',
        facebook: 'Facebook',
        whatsapp: 'WhatsApp',
        email: 'Email'
    };

    const message = `Shared to ${platformNames[platform] || platform} successfully!`;
    showNotification(message, 'success');
}

function showShareError(message) {
    showNotification(message, 'error');
}

function showNotification(message, type) {
    // Remove any existing notifications
    const existingNotifications = document.querySelectorAll('.share-notification');
    existingNotifications.forEach(notification => {
        notification.remove();
    });

    // Create notification element
    const notification = document.createElement('div');
    notification.className = `share-notification ${type}`;
    notification.innerHTML = `
        <div class="notification-content">
            <i class="fa-solid ${type === 'success' ? 'fa-check-circle' : 'fa-exclamation-circle'}"></i>
            <span>${message}</span>
        </div>
    `;

    // Add to page
    document.body.appendChild(notification);

    // Show notification
    setTimeout(() => notification.classList.add('show'), 100);

    // Hide notification after 3 seconds
    setTimeout(() => {
        notification.classList.remove('show');
        setTimeout(() => {
            if (notification.parentNode) {
                notification.parentNode.removeChild(notification);
            }
        }, 300);
    }, 3000);
}

// Close dropdowns when clicking outside
document.addEventListener('click', function (e) {
    if (!e.target.closest('.btn-share')) {
        document.querySelectorAll('.share-dropdown').forEach(dropdown => {
            dropdown.style.display = 'none';
            dropdown.classList.remove('show');
        });
    }
});

// Handle page navigation - reinitialize share buttons for dynamically loaded content
let lastURL = location.href;
new MutationObserver(() => {
    const url = location.href;
    if (url !== lastURL) {
        lastURL = url;
        // Reinitialize share buttons after navigation
        setTimeout(() => {
            addShareButtons();
        }, 500);
    }
}).observe(document, { subtree: true, childList: true });

// Debug function
window.debugSocialIntegration = function () {
    console.log('=== Social Integration Debug Info ===');
    console.log('Share buttons found:', document.querySelectorAll('.btn-share').length);
    console.log('Enhanced share buttons:', document.querySelectorAll('.btn-share.enhanced').length);
    console.log('Share dropdowns:', document.querySelectorAll('.share-dropdown').length);

    document.querySelectorAll('.btn-share').forEach((btn, index) => {
        console.log(`Button ${index}:`, {
            contentType: btn.dataset.contentType,
            contentId: btn.dataset.contentId,
            enhanced: btn.classList.contains('enhanced'),
            hasDropdown: !!btn.querySelector('.share-dropdown')
        });
    });
    console.log('=== End Debug Info ===');
};