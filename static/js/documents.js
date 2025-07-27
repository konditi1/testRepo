document.addEventListener('DOMContentLoaded', function() {
    // Document Repository toggle functionality
    const documentSectionHeader = document.querySelector('.sidebar-section-header');
    const documentToggleIcon = document.querySelector('.document-toggle-icon');
    const documentContent = document.querySelector('.document-repository-content');
    
    if (documentSectionHeader && documentToggleIcon && documentContent) {
        // Set initial state (expanded by default on desktop, collapsed on mobile)
        const isMobile = window.innerWidth <= 768;
        
        if (isMobile) {
            documentToggleIcon.classList.add('collapsed');
            documentContent.classList.remove('expanded');
            documentContent.style.maxHeight = '0';
        } else {
            documentToggleIcon.classList.remove('collapsed');
            documentContent.classList.add('expanded');
            documentContent.style.maxHeight = '800px';
        }
        
        // Toggle functionality
        documentSectionHeader.addEventListener('click', function() {
            // Toggle classes
            documentContent.classList.toggle('expanded');
            documentToggleIcon.classList.toggle('collapsed');
            
            // Force redraw by triggering reflow
            void documentContent.offsetWidth;
            
            // Set appropriate max-height based on expanded state
            if (documentContent.classList.contains('expanded')) {
                documentContent.style.maxHeight = '800px';
            } else {
                documentContent.style.maxHeight = '0';
            }
            
            // Save user preference in localStorage
            localStorage.setItem('documentExpanded', documentContent.classList.contains('expanded'));
        });
        
        // Check localStorage for user preference
        const documentPreference = localStorage.getItem('documentExpanded');
        if (documentPreference !== null && !isMobile) {
            if (documentPreference === 'true') {
                documentContent.classList.add('expanded');
                documentToggleIcon.classList.remove('collapsed');
                documentContent.style.maxHeight = '800px';
            } else {
                documentContent.classList.remove('expanded');
                documentToggleIcon.classList.add('collapsed');
                documentContent.style.maxHeight = '0';
            }
        }
    }
    
    // Load recent documents for the sidebar
    function loadRecentDocuments() {
        const recentDocsContainer = document.getElementById('recent-documents-container');
        if (!recentDocsContainer) return;
        
        fetch('/documents?format=json&limit=5')
            .then(response => response.json())
            .then(data => {
                if (data.documents && data.documents.length > 0) {
                    let html = '';
                    data.documents.forEach(doc => {
                        html += `
                        <a href="/view-document?id=${doc.id}" class="document-item">
                            <div class="document-icon">
                                ${getDocumentIconByType(doc.file_type)}
                            </div>
                            <div class="document-info">
                                <div class="document-title">${doc.title}</div>
                                <div class="document-meta">
                                    <span class="document-author">${doc.username}</span>
                                    <span class="document-date">${doc.created_at_human}</span>
                                </div>
                            </div>
                        </a>`;
                    });
                    recentDocsContainer.innerHTML = html;
                    
                    // Update document stats
                    const totalDocsElement = document.getElementById('total-documents');
                    const totalDownloadsElement = document.getElementById('total-downloads');
                    
                    if (totalDocsElement) totalDocsElement.textContent = data.total_documents || '0';
                    if (totalDownloadsElement) totalDownloadsElement.textContent = data.total_downloads || '0';
                } else {
                    recentDocsContainer.innerHTML = '<p class="no-documents">No documents available</p>';
                }
            })
            .catch(error => {
                console.error('Error loading recent documents:', error);
                recentDocsContainer.innerHTML = '<p class="error">Failed to load documents</p>';
            });
    }
    
    // Helper function to get document icon based on file type
    function getDocumentIconByType(fileType) {
        switch (fileType) {
            case 'pdf':
                return '<i class="fa-regular fa-file-pdf document-icon-pdf"></i>';
            case 'word':
                return '<i class="fa-regular fa-file-word document-icon-word"></i>';
            case 'excel':
                return '<i class="fa-regular fa-file-excel document-icon-excel"></i>';
            case 'powerpoint':
                return '<i class="fa-regular fa-file-powerpoint document-icon-powerpoint"></i>';
            case 'image':
                return '<i class="fa-regular fa-file-image document-icon-image"></i>';
            case 'video':
                return '<i class="fa-regular fa-file-video document-icon-video"></i>';
            case 'audio':
                return '<i class="fa-regular fa-file-audio document-icon-audio"></i>';
            case 'archive':
                return '<i class="fa-regular fa-file-archive document-icon-archive"></i>';
            default:
                return '<i class="fa-regular fa-file document-icon-other"></i>';
        }
    }
    
    // Document upload modal functionality
    const modal = document.getElementById('document-upload-modal');
    const openModalBtn = document.getElementById('open-document-upload');
    const closeModalBtn = document.querySelector('.close');
    
    if (modal && openModalBtn && closeModalBtn) {
        openModalBtn.addEventListener('click', function() {
            modal.style.display = 'block';
        });
        
        closeModalBtn.addEventListener('click', function() {
            modal.style.display = 'none';
        });
        
        window.addEventListener('click', function(event) {
            if (event.target === modal) {
                modal.style.display = 'none';
            }
        });
    }
    
    // Document actions (like, dislike, download)
    document.addEventListener('click', function(e) {
        // Like document
        if (e.target.closest('.btn-like-document')) {
            const button = e.target.closest('.btn-like-document');
            const documentId = button.getAttribute('data-id');
            
            fetch(`/like-document?id=${documentId}`)
                .then(response => response.json())
                .then(data => {
                    const likesElement = button.querySelector('span');
                    const dislikeButton = button.parentElement.querySelector('.btn-dislike-document');
                    const dislikesElement = dislikeButton.querySelector('span');
                    
                    likesElement.textContent = data.likes;
                    dislikesElement.textContent = data.dislikes;
                })
                .catch(error => console.error('Error liking document:', error));
        }
        
        // Dislike document
        if (e.target.closest('.btn-dislike-document')) {
            const button = e.target.closest('.btn-dislike-document');
            const documentId = button.getAttribute('data-id');
            
            fetch(`/dislike-document?id=${documentId}`)
                .then(response => response.json())
                .then(data => {
                    const dislikesElement = button.querySelector('span');
                    const likeButton = button.parentElement.querySelector('.btn-like-document');
                    const likesElement = likeButton.querySelector('span');
                    
                    likesElement.textContent = data.likes;
                    dislikesElement.textContent = data.dislikes;
                })
                .catch(error => console.error('Error disliking document:', error));
        }
        
        // Document download
        if (e.target.closest('.btn-download')) {
            const button = e.target.closest('.btn-download');
            const fileUrl = button.getAttribute('data-url');
            
            if (fileUrl) {
                window.open(fileUrl, '_blank');
            }
        }
        
        // Options menu toggle
        if (e.target.closest('.btn-options')) {
            const button = e.target.closest('.btn-options');
            const dropdown = button.nextElementSibling;
            
            dropdown.style.display = dropdown.style.display === 'none' ? 'block' : 'none';
            
            // Close other open dropdowns
            document.querySelectorAll('.options-dropdown').forEach(menu => {
                if (menu !== dropdown) {
                    menu.style.display = 'none';
                }
            });
            
            e.stopPropagation();
        }
    });
    
    // Close option menus when clicking elsewhere
    document.addEventListener('click', function() {
        document.querySelectorAll('.options-dropdown').forEach(menu => {
            menu.style.display = 'none';
        });
    });
    
    // Load recent documents when page loads
    loadRecentDocuments();
});