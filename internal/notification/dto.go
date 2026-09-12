// Package notification is the monolith-side client for
// notification-service's NotificationService.SendTransactional — the
// synchronous/urgent path, used sparingly, that complements the async
// RabbitMQ notifications.email queue used for ordinary confirmation email
// (see docs/architecture.md's messaging section).
package notification

// Templates are the only vocabulary the monolith and notification-service
// share — deliberately just a name and a data bag, so notification-service
// owns 100% of the actual copy/formatting.
const (
	TemplateBookingApproved = "booking_approved"
	TemplateBookingDisputed = "booking_disputed"
)
