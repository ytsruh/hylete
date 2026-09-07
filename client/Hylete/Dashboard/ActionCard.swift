import SwiftUI

/// Tappable rounded card that mirrors the web's
/// `components.ActionCard`. Used on the dashboard for the
/// "Add Set" and "Timer" shortcuts. The whole card is the
/// hit target; the inner HStack is purely visual so the
/// icon and label share a single tap surface.
struct ActionCard: View {
    let label: String
    let systemImage: String
    let action: () -> Void

    @State private var isPressed: Bool = false

    var body: some View {
        Button(action: action) {
            HStack(spacing: DSSpacing.sm) {
                Image(systemName: systemImage)
                    .font(.title3.weight(.semibold))
                    .foregroundStyle(DSColors.accent)
                Text(label)
                    .font(.headline)
                    .foregroundStyle(DSColors.text)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, DSSpacing.md)
            .padding(.horizontal, DSSpacing.sm)
            .background(
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .fill(DSColors.surface)
            )
            .overlay(
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .stroke(DSColors.separator, lineWidth: 0.5)
            )
            .scaleEffect(isPressed ? 0.97 : 1)
            .animation(.easeOut(duration: 0.1), value: isPressed)
        }
        .buttonStyle(.plain)
        .simultaneousGesture(
            DragGesture(minimumDistance: 0)
                .onChanged { _ in if !isPressed { isPressed = true } }
                .onEnded { _ in isPressed = false }
        )
    }
}

#Preview {
    HStack {
        ActionCard(label: "Add Set", systemImage: Icons.addSet) {}
        ActionCard(label: "Timer", systemImage: Icons.timer) {}
    }
    .padding()
    .background(DSColors.background)
}
