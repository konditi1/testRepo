// email validation
function validateEmail(email) {
    const emailRegex = /^[a-zA-Z0-9._-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/;
    return emailRegex.test(email);
}

document.addEventListener("DOMContentLoaded", () => {
    const emailInput = document.getElementById("email");
    const emailError = document.createElement("div");
    emailError.className = "email-error";
    
    if (emailInput) {
        emailInput.after(emailError);
        
        emailInput.addEventListener("input", function() {
            if (!validateEmail(this.value)) {
                emailError.textContent = "Please enter a valid email address";
                emailError.style.color = "#ff4444";
                emailInput.style.borderColor = "#ff4444";
            } else {
                emailError.textContent = "";
                emailInput.style.borderColor = "#4CAF50";
            }
        });
    }
}); 