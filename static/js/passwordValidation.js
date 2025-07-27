// Password validation
document.addEventListener("DOMContentLoaded", () => {
    // Only run on signup page
    if (window.location.pathname === "/signup") {
        const passwordInput = document.getElementById("password");
        if (passwordInput) { // Check if the password field exists on the page
            console.log("Password input detected!");
            const strengthMeter = document.getElementById("strength-meter");
            const strengthText = document.getElementById("strength-text");
            if (!strengthMeter || !strengthText) {
                console.error("Strength meter or text not found!");
            }

            passwordInput.addEventListener("input", function () {
                const password = this.value;
                let strength = 0;

                const regex = {
                    length: /.{8,}/,
                    upper: /[A-Z]/,
                    lower: /[a-z]/,
                    number: /\d/,
                    special: /[!@#$%^&*(),.?":{}|<>]/,
                };

                if (regex.length.test(password)) strength++;
                if (regex.upper.test(password)) strength++;
                if (regex.lower.test(password)) strength++;
                if (regex.number.test(password)) strength++;
                if (regex.special.test(password)) strength++;

                const meterWidth = (strength / 5) * 100;
                strengthMeter.style.width = meterWidth + "%";

                if (strength === 1) {
                    strengthText.textContent = "Weak";
                    strengthMeter.style.backgroundColor = "red";
                } else if (strength === 2) {
                    strengthText.textContent = "Fair";
                    strengthMeter.style.backgroundColor = "orange";
                } else if (strength === 3) {
                    strengthText.textContent = "Good";
                    strengthMeter.style.backgroundColor = "yellow";
                } else if (strength === 4) {
                    strengthText.textContent = "Strong";
                    strengthMeter.style.backgroundColor = "green";
                } else if (strength === 5) {
                    strengthText.textContent = "Very Strong";
                    strengthMeter.style.backgroundColor = "darkgreen";
                }
            });
        }
    }
});
